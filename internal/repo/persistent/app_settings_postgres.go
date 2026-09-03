package persistent

import (
	"context"
	"errors"
	"fmt"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

// AppSettingsRepo stores the single global settings row in postgres.
type AppSettingsRepo struct {
	*postgres.Postgres
}

// NewAppSettingsRepo creates a new app settings repository.
func NewAppSettingsRepo(pg *postgres.Postgres) *AppSettingsRepo {
	return &AppSettingsRepo{Postgres: pg}
}

// Get returns the settings row, or (zero, false, nil) when none exists yet.
func (r *AppSettingsRepo) Get(ctx context.Context) (entity.AppSettings, bool, error) {
	sql, args, err := r.Builder.
		Select("COALESCE(sync_interval, '')", "COALESCE(message_style::text, '{}')",
			"COALESCE(ingest_api_key, '')", "updated_at").
		From("app_settings").
		Where("id = ?", true).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.AppSettings{}, false, fmt.Errorf("AppSettingsRepo - Get - r.Builder: %w", err)
	}
	var s entity.AppSettings
	var styleJSON string
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&s.SyncInterval, &styleJSON, &s.IngestAPIKey, &s.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.AppSettings{}, false, nil
		}
		return entity.AppSettings{}, false, fmt.Errorf("AppSettingsRepo - Get - QueryRow: %w", err)
	}
	style, perr := entity.ParseMessageStyle(styleJSON)
	if perr != nil {
		return entity.AppSettings{}, false, fmt.Errorf("AppSettingsRepo - Get - message_style: %w", perr)
	}
	s.MessageStyle = style
	return s, true, nil
}

// Upsert creates or updates the single settings row.
func (r *AppSettingsRepo) Upsert(ctx context.Context, s entity.AppSettings) (entity.AppSettings, error) {
	sql, args, err := r.Builder.
		Insert("app_settings").
		Columns("id", "sync_interval", "message_style", "ingest_api_key").
		Values(true, s.SyncInterval, sq.Expr("?::jsonb", s.MessageStyle.JSON()), nullableString(s.IngestAPIKey)).
		Suffix("ON CONFLICT (id) DO UPDATE SET sync_interval = EXCLUDED.sync_interval, " +
			"message_style = EXCLUDED.message_style, ingest_api_key = EXCLUDED.ingest_api_key, updated_at = NOW() " +
			"RETURNING COALESCE(sync_interval, ''), COALESCE(message_style::text, '{}'), COALESCE(ingest_api_key, ''), updated_at").
		ToSql()
	if err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsRepo - Upsert - r.Builder: %w", err)
	}
	var out entity.AppSettings
	var outStyle string
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&out.SyncInterval, &outStyle, &out.IngestAPIKey, &out.UpdatedAt); err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsRepo - Upsert - QueryRow: %w", err)
	}
	style, perr := entity.ParseMessageStyle(outStyle)
	if perr != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsRepo - Upsert - message_style: %w", perr)
	}
	out.MessageStyle = style
	return out, nil
}
