package settings

import (
	"context"
	"fmt"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

const defaultGuildID = "_default_"

// UseCase resolves runtime settings with db overrides and env fallbacks.
type UseCase struct {
	cfg  *config.Config
	repo repo.ScheduleSettingsRepo
}

// New creates a settings usecase.
func New(cfg *config.Config, scheduleRepo repo.ScheduleSettingsRepo) *UseCase {
	return &UseCase{cfg: cfg, repo: scheduleRepo}
}

// NormalizeGuildID maps blank guild IDs to a stable key.
func NormalizeGuildID(guildID string) string {
	if guildID == "" {
		return defaultGuildID
	}
	return guildID
}

// GetEffectiveSchedule returns the merged schedule value for guildID.
func (uc *UseCase) GetEffectiveSchedule(ctx context.Context, guildID string) (entity.EffectiveScheduleSettings, error) {
	guildID = NormalizeGuildID(guildID)
	row, found, err := uc.repo.GetByGuild(ctx, guildID)
	if err != nil {
		return entity.EffectiveScheduleSettings{}, fmt.Errorf("SettingsUseCase - GetEffectiveSchedule - repo.GetByGuild: %w", err)
	}
	if !found {
		row.GuildID = guildID
	}
	return EffectiveSchedule(uc.cfg, row), nil
}

// UpsertSchedule updates per-guild schedule settings.
func (uc *UseCase) UpsertSchedule(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error) {
	cfg.GuildID = NormalizeGuildID(cfg.GuildID)
	out, err := uc.repo.Upsert(ctx, cfg)
	if err != nil {
		return entity.DiscordScheduleSettings{}, fmt.Errorf("SettingsUseCase - UpsertSchedule - repo.Upsert: %w", err)
	}
	return out, nil
}

// EffectiveSchedule resolves db value first, then falls back to env config.
func EffectiveSchedule(cfg *config.Config, db entity.DiscordScheduleSettings) entity.EffectiveScheduleSettings {
	out := entity.EffectiveScheduleSettings{
		GuildID:               NormalizeGuildID(db.GuildID),
		SendChannelID:         cfg.Discord.SendChannelID,
		SendInterval:          cfg.Discord.SendInterval,
		SendHistorySize:       cfg.Discord.SendHistorySize,
		SourceSendChannelID:   "env",
		SourceSendInterval:    "env",
		SourceSendHistorySize: "env",
	}
	if db.SendChannelID != "" {
		out.SendChannelID = db.SendChannelID
		out.SourceSendChannelID = "db"
	}
	if db.SendInterval != "" {
		out.SendInterval = db.SendInterval
		out.SourceSendInterval = "db"
	}
	if db.SendHistorySize > 0 {
		out.SendHistorySize = db.SendHistorySize
		out.SourceSendHistorySize = "db"
	}
	if d, err := time.ParseDuration(out.SendInterval); err == nil {
		out.SendIntervalDuration = d
	}
	return out
}
