// Package appsettings implements the global runtime settings use case.
package appsettings

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
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
	if _, err := time.ParseDuration(interval); err != nil {
		return entity.AppSettings{}, fmt.Errorf("sync_interval must be a valid duration (e.g. 1h): %w", err)
	}
	out, err := uc.repo.Upsert(ctx, entity.AppSettings{SyncInterval: interval})
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
