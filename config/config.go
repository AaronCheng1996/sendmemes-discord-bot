package config

import (
	"fmt"
	"net/url"

	"github.com/caarlos0/env/v11"
)

type (
	// Config -.
	Config struct {
		App     App
		HTTP    HTTP
		Log     Log
		PG      PG
		Admin   Admin
		Discord Discord
		PCloud  PCloud
		Metrics Metrics
		Swagger Swagger
	}

	// App -.
	App struct {
		Name    string `env:"SENDMEMES_APP_NAME,required"`
		Version string `env:"SENDMEMES_APP_VERSION,required"`
	}

	// HTTP -.
	HTTP struct {
		Port           string `env:"SENDMEMES_HTTP_PORT,required"`
		UsePreforkMode bool   `env:"SENDMEMES_HTTP_USE_PREFORK_MODE" envDefault:"false"`
		PublicURL      string `env:"SENDMEMES_HTTP_PUBLIC_URL" envDefault:"http://localhost:8080"` // base URL for Discord embed image resolution
	}

	// Log -.
	Log struct {
		Level string `env:"SENDMEMES_LOG_LEVEL,required"`
	}

	// PG -.
	PG struct {
		PoolMax  int    `env:"SENDMEMES_PG_POOL_MAX,required"`
		URL      string `env:"SENDMEMES_PG_URL"` // optional; if empty, built from SENDMEMES_POSTGRES_* below
		User     string `env:"SENDMEMES_POSTGRES_USER"`
		Password string `env:"SENDMEMES_POSTGRES_PASSWORD"`
		Host     string `env:"SENDMEMES_POSTGRES_HOST" envDefault:"localhost"`
		Port     string `env:"SENDMEMES_POSTGRES_PORT" envDefault:"5432"`
		DB       string `env:"SENDMEMES_POSTGRES_DB" envDefault:"sendmemes"`
		SSLMode  string `env:"SENDMEMES_POSTGRES_SSL_MODE" envDefault:"disable"`
	}

	// Discord -.
	Discord struct {
		Token         string `env:"SENDMEMES_DISCORD_TOKEN,required"`
		ApplicationID string `env:"SENDMEMES_DISCORD_APPLICATION_ID"`
		GuildID       string `env:"SENDMEMES_DISCORD_GUILD_ID"`
		// SendChannelID is the channel for scheduled periodic album sends.
		SendChannelID string `env:"SENDMEMES_DISCORD_CHANNEL_ID"`
		// NotifyChannelID is the channel for "new content discovered" sync
		// notifications. Empty disables them. Per-guild DB override lives in
		// discord_schedule_settings.notify_channel_id.
		NotifyChannelID string `env:"SENDMEMES_DISCORD_NOTIFY_CHANNEL_ID"`
		// SendInterval is how often to push a random album (Go duration string, e.g. "6h").
		SendInterval string `env:"SENDMEMES_DISCORD_SEND_INTERVAL" envDefault:"6h"`
		// SendHistorySize is the number of most-recently-sent albums to exclude from
		// the next scheduled send.  When total albums ≤ SendHistorySize, the history
		// resets automatically (all albums become eligible again).  Default 10.
		SendHistorySize int `env:"SENDMEMES_DISCORD_SEND_HISTORY_SIZE" envDefault:"10"`
		// VerboseLog enables info-level logging for every bot request and scheduled
		// send (received, completed, per-batch progress for full_album).
		VerboseLog bool `env:"SENDMEMES_DISCORD_VERBOSE_LOG" envDefault:"true"`
		// AlbumDefaultSendMode is the send_mode assigned to albums created by the
		// pCloud sync and to admin CreateAlbum calls that omit a mode. Validated at
		// startup; one of Order/Random/Single/Video/Custom.
		AlbumDefaultSendMode string `env:"SENDMEMES_ALBUM_DEFAULT_SEND_MODE" envDefault:"Random"`
	}

	// Admin controls privileged API access.
	Admin struct {
		APIKey string `env:"SENDMEMES_ADMIN_API_KEY"`
	}

	// PCloud holds credentials and settings for the pCloud integration.
	// Auth priority: SENDMEMES_PCLOUD_ACCESS_TOKEN > SENDMEMES_PCLOUD_USERNAME + SENDMEMES_PCLOUD_PASSWORD.
	// Note: pCloud does not support 2FA via API. Disable 2FA on the account if using username/password.
	PCloud struct {
		AccessToken string `env:"SENDMEMES_PCLOUD_ACCESS_TOKEN"`
		// TokenType selects how AccessToken is sent to pCloud:
		//   "session" (default) — sent as auth=   (pcauth cookie / userinfo getauth)
		//   "oauth"             — sent as access_token=  (token from a registered pCloud OAuth app)
		TokenType string `env:"SENDMEMES_PCLOUD_TOKEN_TYPE" envDefault:"session"`
		Username  string `env:"SENDMEMES_PCLOUD_USERNAME"`
		Password  string `env:"SENDMEMES_PCLOUD_PASSWORD"`
		// RootFolderIDs is a comma-separated list of pCloud folder IDs to sync (e.g. "26096342557,26083978164").
		RootFolderIDs []int64 `env:"SENDMEMES_CLOUD_MAIN_FOLDER_ID"`
		// APIEndpoint is the pCloud REST base URL. Use https://eapi.pcloud.com for EU accounts.
		APIEndpoint  string `env:"SENDMEMES_PCLOUD_API_ENDPOINT" envDefault:"https://api.pcloud.com"`
		SyncInterval string `env:"SENDMEMES_PCLOUD_SYNC_INTERVAL" envDefault:"1h"`
	}

	// Metrics -.
	Metrics struct {
		Enabled bool `env:"SENDMEMES_METRICS_ENABLED" envDefault:"true"`
	}

	// Swagger -.
	Swagger struct {
		Enabled bool `env:"SENDMEMES_SWAGGER_ENABLED" envDefault:"false"`
	}
)

// NewConfig returns app config.
func NewConfig() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	if cfg.PG.URL == "" {
		if cfg.PG.User == "" || cfg.PG.Password == "" {
			return nil, fmt.Errorf("config error: when SENDMEMES_PG_URL is not set, SENDMEMES_POSTGRES_USER and SENDMEMES_POSTGRES_PASSWORD are required")
		}
		cfg.PG.URL = buildPGURL(&cfg.PG)
	}

	return cfg, nil
}

// buildPGURL builds postgres connection URL from SENDMEMES_POSTGRES_* env (shared_env style).
func buildPGURL(pg *PG) string {
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(pg.User, pg.Password),
		Host:     pg.Host + ":" + pg.Port,
		Path:     "/" + pg.DB,
		RawQuery: "sslmode=" + url.QueryEscape(pg.SSLMode),
	}
	return u.String()
}
