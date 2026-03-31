// Package app configures and runs application.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/discord"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/persistent"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/webapi"
	adminuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/admin"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/images"
	settingsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/settings"
	syncuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/sync"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/translation"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/httpserver"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// Run creates objects via constructors.
func Run(cfg *config.Config) { //nolint: gocyclo,cyclop,funlen,gocritic,nolintlint
	l := logger.New(cfg.Log.Level)

	// Repository
	pg, err := postgres.New(cfg.PG.URL, postgres.MaxPoolSize(cfg.PG.PoolMax))
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - postgres.New: %w", err))
	}
	defer pg.Close()

	// Use-Case: translation
	translationUseCase := translation.New(
		persistent.New(pg),
		webapi.New(),
	)

	// Repos: images & albums
	imagesRepo := persistent.NewImagesRepo(pg)
	albumsRepo := persistent.NewAlbumsRepo(pg)
	scheduleSettingsRepo := persistent.NewScheduleSettingsRepo(pg)

	// pCloud client + sync use case
	pcloudClient := webapi.NewPCloudClient(
		cfg.PCloud.AccessToken,
		cfg.PCloud.Username,
		cfg.PCloud.Password,
		cfg.PCloud.APIEndpoint,
	)
	// Authenticate once at startup (no-op if access token already set).
	if err = pcloudClient.Login(context.Background()); err != nil {
		l.Fatal(fmt.Errorf("app - Run - pcloudClient.Login: %w", err))
	}
	syncUseCase := syncuc.New(pcloudClient, albumsRepo, imagesRepo, cfg.PCloud.RootFolderID)

	// Use-Case: images
	imagesUseCase := images.New(imagesRepo, albumsRepo, pcloudClient, cfg.HTTP.PublicURL)
	settingsUseCase := settingsuc.New(cfg, scheduleSettingsRepo)
	adminUseCase := adminuc.New(albumsRepo, imagesRepo, settingsUseCase)

	// Discord Bot
	discordBot, err := discord.NewBot(cfg, l, translationUseCase, imagesUseCase, syncUseCase, settingsUseCase)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - discord.NewBot: %w", err))
	}
	discordBot.Start()

	// HTTP Server (REST API)
	httpServer := httpserver.New(l, httpserver.Port(cfg.HTTP.Port), httpserver.Prefork(cfg.HTTP.UsePreforkMode))
	restapi.NewRouter(httpServer.App, cfg, translationUseCase, adminUseCase, l)
	httpServer.Start()

	// Waiting signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	select {
	case s := <-interrupt:
		l.Info("app - Run - signal: %s", s.String())
	case err = <-httpServer.Notify():
		l.Error(fmt.Errorf("app - Run - httpServer.Notify: %w", err))
	}

	// Shutdown
	if err = discordBot.Close(); err != nil {
		l.Error(fmt.Errorf("app - Run - discordBot.Close: %w", err))
	}
	if err = httpServer.Shutdown(); err != nil {
		l.Error(fmt.Errorf("app - Run - httpServer.Shutdown: %w", err))
	}
}
