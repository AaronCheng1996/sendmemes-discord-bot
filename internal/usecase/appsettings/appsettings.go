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
	repo             repo.AppSettingsRepo
	defaultInterval  string
	defaultIngestKey string
}

// New creates an app-settings use case. defaultInterval is the env fallback
// (PCLOUD_SYNC_INTERVAL) and defaultIngestKey the env fallback (INGEST_API_KEY),
// each used until a value is stored.
func New(r repo.AppSettingsRepo, defaultInterval, defaultIngestKey string) *UseCase {
	return &UseCase{repo: r, defaultInterval: defaultInterval, defaultIngestKey: defaultIngestKey}
}

// GetIngestAPIKey returns the stored ingest credential, falling back to the env
// value while none is stored. The fallback is what keeps an existing deployment
// working after this became a database setting.
func (uc *UseCase) GetIngestAPIKey(ctx context.Context) (string, error) {
	s, found, err := uc.repo.Get(ctx)
	if err != nil {
		return "", fmt.Errorf("AppSettingsUseCase - GetIngestAPIKey - repo.Get: %w", err)
	}
	if !found || strings.TrimSpace(s.IngestAPIKey) == "" {
		return uc.defaultIngestKey, nil
	}

	return s.IngestAPIKey, nil
}

// SetIngestAPIKey stores a new ingest credential. An empty key clears the stored
// one, which falls back to the env value rather than disabling the endpoint —
// clearing a field should not silently lock out a client that still has a key.
func (uc *UseCase) SetIngestAPIKey(ctx context.Context, key string) error {
	// Read-modify-write: the row also holds the interval and message defaults.
	current, err := uc.Get(ctx)
	if err != nil {
		return err
	}
	current.IngestAPIKey = strings.TrimSpace(key)

	if _, err = uc.repo.Upsert(ctx, current); err != nil {
		return fmt.Errorf("AppSettingsUseCase - SetIngestAPIKey - repo.Upsert: %w", err)
	}

	return nil
}

// HasIngestAPIKey reports whether any key is in force, stored or from env. It is
// what the dashboard is told; the key itself never leaves the server.
func (uc *UseCase) HasIngestAPIKey(ctx context.Context) (bool, error) {
	key, err := uc.GetIngestAPIKey(ctx)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(key) != "", nil
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
	if _, err := uc.repo.Upsert(ctx, entity.AppSettings{
		SyncInterval: uc.defaultInterval,
		IngestAPIKey: uc.defaultIngestKey,
	}); err != nil {
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
