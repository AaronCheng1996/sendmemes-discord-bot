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
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/localfs"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/persistent"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo/webapi"
	adminuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/admin"
	appsettingsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/appsettings"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/images"
	jobsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/jobs"
	rulesuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/rules"
	syncuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/sync"
	taskrunsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/taskruns"
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

	// Repos: images & albums
	imagesRepo := persistent.NewImagesRepo(pg)
	albumsRepo := persistent.NewAlbumsRepo(pg)
	deliveryRulesRepo := persistent.NewDeliveryRulesRepo(pg)
	appSettingsRepo := persistent.NewAppSettingsRepo(pg)
	adminAuditRepo := persistent.NewAdminAuditRepo(pg)
	syncEventsRepo := persistent.NewSyncEventsRepo(pg)
	taskRunsRepo := persistent.NewTaskRunsRepo(pg)
	systemRepo := persistent.NewSystemRepo(pg)

	// MediaSource (pCloud or local filesystem) + sync use case
	mediaSource, mediaSourceName, err := buildMediaSource(cfg)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - buildMediaSource: %w", err))
	}
	// Validate the configured default album send mode once, fail fast on garbage.
	defaultSendMode, err := entity.ParseAlbumSendMode(cfg.Discord.AlbumDefaultSendMode)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - invalid ALBUM_DEFAULT_SEND_MODE: %w", err))
	}
	syncUseCase := syncuc.New(mediaSource, mediaSourceName, albumsRepo, imagesRepo, syncEventsRepo, defaultSendMode)

	// Use-Case: images, delivery rules, app settings
	imagesUseCase := images.New(imagesRepo, albumsRepo, mediaSource, cfg.HTTP.PublicURL)
	rulesUseCase := rulesuc.New(deliveryRulesRepo)
	appSettingsUseCase := appsettingsuc.New(appSettingsRepo, cfg.PCloud.SyncInterval, cfg.Ingest.APIKey)
	taskRunsUseCase := taskrunsuc.New(taskRunsRepo)

	// Seed env-derived defaults once (no-op when rows already exist).
	seedCtx := context.Background()
	if err = appSettingsUseCase.EnsureSeeded(seedCtx); err != nil {
		l.Error(fmt.Errorf("app - Run - appSettings seed: %w", err))
	}
	if err = rulesUseCase.EnsureSeeded(seedCtx, defaultRulesFromEnv(cfg)); err != nil {
		l.Error(fmt.Errorf("app - Run - rules seed: %w", err))
	}

	// Discord Bot
	discordBot, err := discord.NewBot(cfg, l, imagesUseCase, syncUseCase, rulesUseCase, appSettingsUseCase, taskRunsUseCase)
	if err != nil {
		l.Fatal(fmt.Errorf("app - Run - discord.NewBot: %w", err))
	}
	discordBot.Start()
	jobsManager := jobsuc.New()
	adminUseCase := adminuc.New(albumsRepo, imagesRepo, imagesUseCase, rulesUseCase, appSettingsUseCase, adminAuditRepo, syncEventsRepo, taskRunsUseCase, systemRepo, discordBot, jobsManager, defaultSendMode)

	// HTTP Server (REST API)
	httpServer := httpserver.New(l, httpserver.Port(cfg.HTTP.Port), httpserver.Prefork(cfg.HTTP.UsePreforkMode))
	restapi.NewRouter(httpServer.App, cfg, adminUseCase, taskRunsUseCase, appSettingsUseCase, l)
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

// buildMediaSource wires the MediaSource selected by MEDIA_SOURCE ("pcloud",
// the default, or "local") and returns it alongside its entity.MediaSource*
// label, which the sync use case stamps on synced images and uses to scope
// pruning to rows it owns.
func buildMediaSource(cfg *config.Config) (repo.MediaSource, string, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Media.Source)) {
	case "", entity.MediaSourcePCloud:
		pcloudClient := webapi.NewPCloudClient(
			cfg.PCloud.AccessToken,
			cfg.PCloud.TokenType,
			cfg.PCloud.Username,
			cfg.PCloud.Password,
			cfg.PCloud.APIEndpoint,
			cfg.PCloud.RootFolderIDs,
		)
		// Authenticate once at startup (no-op if access token already set).
		if err := pcloudClient.Login(context.Background()); err != nil {
			return nil, "", fmt.Errorf("pcloudClient.Login: %w", err)
		}
		return pcloudClient, entity.MediaSourcePCloud, nil
	case entity.MediaSourceLocal:
		return localfs.New(cfg.Media.LocalRoot, cfg.HTTP.PublicURL), entity.MediaSourceLocal, nil
	default:
		return nil, "", fmt.Errorf("invalid MEDIA_SOURCE: %s", cfg.Media.Source)
	}
}

// defaultRulesFromEnv builds the seed delivery rules from env configuration:
// a scheduled rule from DISCORD_CHANNEL_ID and new_album/new_files rules from
// DISCORD_NOTIFY_CHANNEL_ID. Only seeded once, when no rules exist yet.
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
