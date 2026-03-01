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
		Discord Discord
		PCloud  PCloud
		Metrics Metrics
		Swagger Swagger
	}

	// App -.
	App struct {
		Name    string `env:"APP_NAME,required"`
		Version string `env:"APP_VERSION,required"`
	}

	// HTTP -.
	HTTP struct {
		Port           string `env:"HTTP_PORT,required"`
		UsePreforkMode bool   `env:"HTTP_USE_PREFORK_MODE" envDefault:"false"`
		PublicURL      string `env:"HTTP_PUBLIC_URL" envDefault:"http://localhost:8080"` // base URL for Discord embed image resolution
	}

	// Log -.
	Log struct {
		Level string `env:"LOG_LEVEL,required"`
	}

	// PG -.
	PG struct {
		PoolMax  int    `env:"PG_POOL_MAX,required"`
		URL      string `env:"PG_URL"` // optional; if empty, built from POSTGRES_* below
		User     string `env:"POSTGRES_USER"`
		Password string `env:"POSTGRES_PASSWORD"`
		Host     string `env:"POSTGRES_HOST" envDefault:"localhost"`
		Port     string `env:"POSTGRES_PORT" envDefault:"5432"`
		DB       string `env:"POSTGRES_DB" envDefault:"sendmemes"`
		SSLMode  string `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	}

	// Discord -.
	Discord struct {
		Token         string `env:"DISCORD_TOKEN,required"`
		ApplicationID string `env:"DISCORD_APPLICATION_ID"`
		GuildID       string `env:"DISCORD_GUILD_ID"`
		// SendChannelID is the channel for scheduled periodic album sends.
		SendChannelID string `env:"DISCORD_CHANNEL_ID"`
		// SendInterval is how often to push a random album (Go duration string, e.g. "6h").
		SendInterval string `env:"DISCORD_SEND_INTERVAL" envDefault:"6h"`
	}

	// PCloud holds credentials and settings for the pCloud integration.
	// Auth priority: PCLOUD_ACCESS_TOKEN > PCLOUD_USERNAME + PCLOUD_PASSWORD.
	PCloud struct {
		AccessToken  string `env:"PCLOUD_ACCESS_TOKEN"`
		Username     string `env:"PCLOUD_USERNAME"`
		Password     string `env:"PCLOUD_PASSWORD"`
		RootFolderID int64  `env:"CLOUD_MAIN_FOLDER_ID" envDefault:"0"`
		// APIEndpoint is the pCloud REST base URL. Use https://eapi.pcloud.com for EU accounts.
		APIEndpoint  string `env:"PCLOUD_API_ENDPOINT" envDefault:"https://api.pcloud.com"`
		SyncInterval string `env:"PCLOUD_SYNC_INTERVAL" envDefault:"1h"`
	}

	// Metrics -.
	Metrics struct {
		Enabled bool `env:"METRICS_ENABLED" envDefault:"true"`
	}

	// Swagger -.
	Swagger struct {
		Enabled bool `env:"SWAGGER_ENABLED" envDefault:"false"`
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
			return nil, fmt.Errorf("config error: when PG_URL is not set, POSTGRES_USER and POSTGRES_PASSWORD are required")
		}
		cfg.PG.URL = buildPGURL(&cfg.PG)
	}

	return cfg, nil
}

// buildPGURL builds postgres connection URL from POSTGRES_* env (shared_env style).
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
