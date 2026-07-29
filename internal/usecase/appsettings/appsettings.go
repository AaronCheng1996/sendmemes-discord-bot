// Package appsettings implements the global runtime settings use case.
package appsettings

import (
	"context"
	"fmt"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/schedulespec"
)

// UseCase resolves global settings, falling back to env-provided defaults.
type UseCase struct {
	repo            repo.AppSettingsRepo
	defaultInterval string
}

// New creates an app-settings use case. defaultInterval is the env fallback
// (PCLOUD_SYNC_INTERVAL) used until a value is stored.
func New(r repo.AppSettingsRepo, defaultInterval string) *UseCase {
	return &UseCase{repo: r, defaultInterval: defaultInterval}
}

// GetSyncInterval returns the stored sync cadence, or the env default when unset.
func (uc *UseCase) GetSyncInterval(ctx context.Context) (string, error) {
	s, found, err := uc.repo.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("AppSettingsUseCase - GetSyncInterval - repo.Get: %w", err)
	}
	if !found || strings.TrimSpace(s.SyncInterval) == "" {
		return uc.defaultInterval, nil
	}
	return s.SyncInterval, nil
}

// SetSyncInterval validates and stores the sync cadence.
func (uc *UseCase) SetSyncInterval(ctx context.Context, interval string) (entity.AppSettings, error) {
	interval = strings.TrimSpace(interval)
	if _, err := schedulespec.Parse(interval); err != nil {
		return entity.AppSettings{}, fmt.Errorf("sync_interval must be a valid duration or cron expression (e.g. 1h or 0 9 * * *): %w", err)
	}
	// Read-modify-write: the row also holds the message defaults, and upserting
	// a bare AppSettings would silently wipe them.
	current, err := uc.Get(ctx)
	if err != nil {
		return entity.AppSettings{}, err
	}
	current.SyncInterval = interval

	out, err := uc.repo.Upsert(ctx, current)
	if err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsUseCase - SetSyncInterval - repo.Upsert: %w", err)
	}
	return out, nil
}

// EnsureSeeded stores the env default once when no row exists yet.
func (uc *UseCase) EnsureSeeded(ctx context.Context) error {
	_, found, err := uc.repo.Get(ctx)
	if err != nil {
		return fmt.Errorf("AppSettingsUseCase - EnsureSeeded - repo.Get: %w", err)
	}
	if found {
		return nil
	}
	if _, err := uc.repo.Upsert(ctx, entity.AppSettings{SyncInterval: uc.defaultInterval}); err != nil {
		return fmt.Errorf("AppSettingsUseCase - EnsureSeeded - repo.Upsert: %w", err)
	}
	return nil
}

// Get returns the full settings row with the env fallback applied to the sync
// interval, so callers always see an effective value.
func (uc *UseCase) Get(ctx context.Context) (entity.AppSettings, error) {
	s, found, err := uc.repo.Get(ctx)
	if err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsUseCase - Get - repo.Get: %w", err)
	}
	if !found || strings.TrimSpace(s.SyncInterval) == "" {
		s.SyncInterval = uc.defaultInterval
	}
	return s, nil
}

// SetMessageDefaults stores the app-wide message presentation defaults — the
// bottom layer that delivery rules and albums override.
func (uc *UseCase) SetMessageDefaults(ctx context.Context, style entity.MessageStyle) (entity.AppSettings, error) {
	current, err := uc.Get(ctx)
	if err != nil {
		return entity.AppSettings{}, err
	}
	style.Title = strings.TrimSpace(style.Title)
	style.Body = strings.TrimSpace(style.Body)
	current.MessageStyle = style

	out, err := uc.repo.Upsert(ctx, current)
	if err != nil {
		return entity.AppSettings{}, fmt.Errorf("AppSettingsUseCase - SetMessageDefaults - repo.Upsert: %w", err)
	}
	return out, nil
}
