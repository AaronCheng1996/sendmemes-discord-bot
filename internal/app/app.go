// Package app configures and runs application.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/discord"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/controller/restapi"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/persistent"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/webapi"
	adminuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/admin"
	appsettingsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/appsettings"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/images"
	rulesuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/rules"
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
	deliveryRulesRepo := persistent.NewDeliveryRulesRepo(pg)
	appSettingsRepo := persistent.NewAppSettingsRepo(pg)
	adminAuditRepo := persistent.NewAdminAuditRepo(pg)
	syncEventsRepo := persistent.NewSyncEventsRepo(pg)
	systemRepo := persistent.NewSystemRepo(pg)

	// pCloud client + sync use case
	pcloudClient := webapi.NewPCloudClient(
		cfg.PCloud.AccessToken,
		cfg.PCloud.TokenType,
		cfg.PCloud.Username,
		cfg.PCloud.Password,
		cfg.PCloud.APIEndpoint,
	)
	// Authenticate once at startup (no-op if access token already set).
	if err = pcloudClient.Login(context.Background()); err != nil {
		l.Fatal(fmt.Errorf("app - Run - pcloudClient.Login: %w", err))
	}
	// Validate the configured default album send mode once, fail fast on garbage.
	defaultSendMode, err := entity.ParseAlbumSendMode(cfg.Discord.AlbumDefaultSendMode)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - invalid SENDMEMES_ALBUM_DEFAULT_SEND_MODE: %w", err))
	}
	syncUseCase := syncuc.New(pcloudClient, albumsRepo, imagesRepo, syncEventsRepo, cfg.PCloud.RootFolderIDs, defaultSendMode)

	// Use-Case: images, delivery rules, app settings
	imagesUseCase := images.New(imagesRepo, albumsRepo, pcloudClient, cfg.HTTP.PublicURL)
	rulesUseCase := rulesuc.New(deliveryRulesRepo)
	appSettingsUseCase := appsettingsuc.New(appSettingsRepo, cfg.PCloud.SyncInterval)

	// Seed env-derived defaults once (no-op when rows already exist).
	seedCtx := context.Background()
	if err = appSettingsUseCase.EnsureSeeded(seedCtx); err != nil {
		l.Error(fmt.Errorf("app - Run - appSettings seed: %w", err))
	}
	if err = rulesUseCase.EnsureSeeded(seedCtx, defaultRulesFromEnv(cfg)); err != nil {
		l.Error(fmt.Errorf("app - Run - rules seed: %w", err))
	}

	// Discord Bot
	discordBot, err := discord.NewBot(cfg, l, translationUseCase, imagesUseCase, syncUseCase, rulesUseCase, appSettingsUseCase)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - discord.NewBot: %w", err))
	}
	discordBot.Start()
	adminUseCase := adminuc.New(albumsRepo, imagesRepo, imagesUseCase, rulesUseCase, appSettingsUseCase, adminAuditRepo, syncEventsRepo, systemRepo, discordBot, defaultSendMode)

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

// defaultRulesFromEnv builds the seed delivery rules from env configuration:
// a scheduled rule from SENDMEMES_DISCORD_CHANNEL_ID and new_album/new_files rules from
// SENDMEMES_DISCORD_NOTIFY_CHANNEL_ID. Only seeded once, when no rules exist yet.
func defaultRulesFromEnv(cfg *config.Config) []entity.DeliveryRule {
	var rules []entity.DeliveryRule
	if strings.TrimSpace(cfg.Discord.SendChannelID) != "" {
		rules = append(rules, entity.DeliveryRule{
			Name:         "Scheduled (env)",
			GuildID:      cfg.Discord.GuildID,
			TriggerType:  entity.TriggerScheduled,
			ChannelID:    cfg.Discord.SendChannelID,
			SendInterval: cfg.Discord.SendInterval,
			HistorySize:  cfg.Discord.SendHistorySize,
			Enabled:      true,
		})
	}
	if strings.TrimSpace(cfg.Discord.NotifyChannelID) != "" {
		rules = append(rules,
			entity.DeliveryRule{
				Name: "New albums (env)", GuildID: cfg.Discord.GuildID,
				TriggerType: entity.TriggerNewAlbum, ChannelID: cfg.Discord.NotifyChannelID, Enabled: true,
			},
			entity.DeliveryRule{
				Name: "New files (env)", GuildID: cfg.Discord.GuildID,
				TriggerType: entity.TriggerNewFiles, ChannelID: cfg.Discord.NotifyChannelID, Enabled: true,
			},
		)
	}
	return rules
}
