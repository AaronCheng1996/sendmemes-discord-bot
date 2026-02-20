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
		Token string `env:"DISCORD_TOKEN,required"`
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
