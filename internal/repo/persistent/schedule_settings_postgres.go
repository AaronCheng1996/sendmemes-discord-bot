package persistent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// ScheduleSettingsRepo stores schedule settings in postgres.
type ScheduleSettingsRepo struct {
	*postgres.Postgres
}

// NewScheduleSettingsRepo creates a new schedule settings repository.
func NewScheduleSettingsRepo(pg *postgres.Postgres) *ScheduleSettingsRepo {
	return &ScheduleSettingsRepo{Postgres: pg}
}

// GetByGuild returns schedule settings for guildID.
func (r *ScheduleSettingsRepo) GetByGuild(ctx context.Context, guildID string) (entity.DiscordScheduleSettings, bool, error) {
	sql, args, err := r.Builder.
		Select("guild_id", "COALESCE(send_channel_id, '')", "COALESCE(send_interval, '')", "COALESCE(send_history_size, 0)", "updated_at").
		From("discord_schedule_settings").
		Where("guild_id = ?", guildID).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.DiscordScheduleSettings{}, false, fmt.Errorf("ScheduleSettingsRepo - GetByGuild - r.Builder: %w", err)
	}

	var cfg entity.DiscordScheduleSettings
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(
		&cfg.GuildID, &cfg.SendChannelID, &cfg.SendInterval, &cfg.SendHistorySize, &cfg.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.DiscordScheduleSettings{}, false, nil
		}
		return entity.DiscordScheduleSettings{}, false, fmt.Errorf("ScheduleSettingsRepo - GetByGuild - QueryRow: %w", err)
	}

	return cfg, true, nil
}

// Upsert creates or updates schedule settings by guild_id.
func (r *ScheduleSettingsRepo) Upsert(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error) {
	sql, args, err := r.Builder.
		Insert("discord_schedule_settings").
		Columns("guild_id", "send_channel_id", "send_interval", "send_history_size").
		Values(cfg.GuildID, cfg.SendChannelID, cfg.SendInterval, cfg.SendHistorySize).
		Suffix("ON CONFLICT (guild_id) DO UPDATE SET send_channel_id = EXCLUDED.send_channel_id, send_interval = EXCLUDED.send_interval, send_history_size = EXCLUDED.send_history_size, updated_at = NOW() RETURNING guild_id, COALESCE(send_channel_id, ''), COALESCE(send_interval, ''), COALESCE(send_history_size, 0), updated_at").
		ToSql()
	if err != nil {
		return entity.DiscordScheduleSettings{}, fmt.Errorf("ScheduleSettingsRepo - Upsert - r.Builder: %w", err)
	}

	var out entity.DiscordScheduleSettings
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(
		&out.GuildID, &out.SendChannelID, &out.SendInterval, &out.SendHistorySize, &out.UpdatedAt,
	); err != nil {
		return entity.DiscordScheduleSettings{}, fmt.Errorf("ScheduleSettingsRepo - Upsert - QueryRow: %w", err)
	}

	return out, nil
}
