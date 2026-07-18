package persistent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
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
		Select("COALESCE(sync_interval, '')", "updated_at").
		From("app_settings").
		Where("id = ?", true).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.AppSettings{}, false, fmt.Errorf("AppSettingsRepo - Get - r.Builder: %w", err)
	}
	var s entity.AppSettings
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&s.SyncInterval, &s.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.AppSettings{}, false, nil
		}
		return entity.AppSettings{}, false, fmt.Errorf("AppSettingsRepo - Get - QueryRow: %w", err)
	}
	return s, true, nil
}

// Upsert creates or updates the single settings row.
func (r *AppSettingsRepo) Upsert(ctx context.Context, s entity.AppSettings) (entity.AppSettings, error) {
	sql, args, err := r.Builder.
		Insert("app_settings").
		Columns("id", "sync_interval").
		Values(true, s.SyncInterval).
		Suffix("ON CONFLICT (id) DO UPDATE SET sync_interval = EXCLUDED.sync_interval, updated_at = NOW() RETURNING COALESCE(sync_interval, ''), updated_at").
		ToSql()
	if err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsRepo - Upsert - r.Builder: %w", err)
	}
	var out entity.AppSettings
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&out.SyncInterval, &out.UpdatedAt); err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsRepo - Upsert - QueryRow: %w", err)
	}
	return out, nil
}
