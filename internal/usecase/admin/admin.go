package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
)

// UseCase provides admin CRUD and settings operations.
type UseCase struct {
	albums   repo.AlbumsRepo
	images   repo.ImagesRepo
	settings usecase.Settings
	audit    repo.AdminAuditRepo
	system   repo.SystemRepo
	runtime  usecase.AdminRuntime
}

// New creates admin usecase.
func New(
	albums repo.AlbumsRepo,
	images repo.ImagesRepo,
	settings usecase.Settings,
	audit repo.AdminAuditRepo,
	system repo.SystemRepo,
	runtime usecase.AdminRuntime,
) *UseCase {
	return &UseCase{
		albums:   albums,
		images:   images,
		settings: settings,
		audit:    audit,
		system:   system,
		runtime:  runtime,
	}
}

func (uc *UseCase) ListAlbums(ctx context.Context, offset, limit int) ([]entity.Album, error) {
	return uc.albums.List(ctx, offset, limit)
}

func (uc *UseCase) GetAlbum(ctx context.Context, id int) (entity.Album, error) {
	return uc.albums.GetByID(ctx, id)
}

func (uc *UseCase) CreateAlbum(ctx context.Context, name string) (entity.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Album{}, fmt.Errorf("album name is required")
	}
	return uc.albums.Create(ctx, name)
}

func (uc *UseCase) UpdateAlbum(ctx context.Context, id int, name string) (entity.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Album{}, fmt.Errorf("album name is required")
	}
	return uc.albums.Update(ctx, id, name)
}

func (uc *UseCase) DeleteAlbum(ctx context.Context, id int) error {
	return uc.albums.Delete(ctx, id)
}

func (uc *UseCase) ListImages(ctx context.Context, albumID, offset, limit int) ([]entity.Image, error) {
	return uc.images.List(ctx, albumID, offset, limit)
}

func (uc *UseCase) GetImage(ctx context.Context, id int) (entity.Image, error) {
	return uc.images.GetByID(ctx, id)
}

func (uc *UseCase) CreateImage(ctx context.Context, img entity.Image) (entity.Image, error) {
	if strings.TrimSpace(img.URL) == "" {
		return entity.Image{}, fmt.Errorf("image url is required")
	}
	return uc.images.Insert(ctx, img)
}

func (uc *UseCase) UpdateImage(ctx context.Context, img entity.Image) (entity.Image, error) {
	if img.ID <= 0 {
		return entity.Image{}, fmt.Errorf("image id is required")
	}
	if strings.TrimSpace(img.URL) == "" {
		return entity.Image{}, fmt.Errorf("image url is required")
	}
	return uc.images.Update(ctx, img)
}

func (uc *UseCase) DeleteImage(ctx context.Context, id int) error {
	return uc.images.Delete(ctx, id)
}

func (uc *UseCase) GetEffectiveSchedule(ctx context.Context, guildID string) (entity.EffectiveScheduleSettings, error) {
	return uc.settings.GetEffectiveSchedule(ctx, guildID)
}

func (uc *UseCase) UpsertSchedule(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error) {
	return uc.settings.UpsertSchedule(ctx, cfg)
}

func (uc *UseCase) RecordAudit(ctx context.Context, actor, action, targetType, targetID string, metadata map[string]any) error {
	if uc.audit == nil {
		return nil
	}
	if actor == "" {
		actor = "api_key"
	}
	return uc.audit.Insert(ctx, entity.AdminAuditLog{
		Actor:      actor,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   metadata,
	})
}

func (uc *UseCase) GetSystemStatus(ctx context.Context, guildID string) (entity.SystemStatus, error) {
	effective, err := uc.settings.GetEffectiveSchedule(ctx, guildID)
	if err != nil {
		return entity.SystemStatus{}, err
	}
	dbStatus := "ok"
	if uc.system != nil {
		if err = uc.system.Ping(ctx); err != nil {
			dbStatus = "fail"
		}
	}
	connected, user := false, ""
	if uc.runtime != nil {
		connected, user = uc.runtime.GetDiscordStatus(ctx)
	}
	return entity.SystemStatus{
		ServerTime:        time.Now().UTC(),
		DatabaseStatus:    dbStatus,
		DiscordConnected:  connected,
		DiscordUser:       user,
		EffectiveSchedule: effective,
	}, nil
}

func (uc *UseCase) TriggerScheduleNow(ctx context.Context, guildID, actor string) (entity.ManualScheduleTriggerResult, error) {
	if uc.runtime == nil {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("runtime trigger is not available")
	}
	res, err := uc.runtime.TriggerScheduleNow(ctx, guildID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "schedule.trigger_now", "schedule", guildID, map[string]any{
		"triggered":  res.Triggered,
		"album_id":   res.AlbumID,
		"album_name": res.AlbumName,
		"channel_id": res.ChannelID,
		"message_id": res.MessageID,
	})
	return res, nil
}
