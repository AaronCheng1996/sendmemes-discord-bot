package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	imagesUC usecase.Images
	settings usecase.Settings
	audit    repo.AdminAuditRepo
	system   repo.SystemRepo
	runtime  usecase.AdminRuntime
}

// New creates admin usecase.
func New(
	albums repo.AlbumsRepo,
	images repo.ImagesRepo,
	imagesUC usecase.Images,
	settings usecase.Settings,
	audit repo.AdminAuditRepo,
	system repo.SystemRepo,
	runtime usecase.AdminRuntime,
) *UseCase {
	return &UseCase{
		albums:   albums,
		images:   images,
		imagesUC: imagesUC,
		settings: settings,
		audit:    audit,
		system:   system,
		runtime:  runtime,
	}
}

// ListAlbums returns paginated albums with preview URLs already resolved.
// Preview rule: cover image (if has_cover && cover_image_id) → first image in album → empty.
// pCloud URLs go through PCloudClient's in-memory cache to limit upstream API calls.
func (uc *UseCase) ListAlbums(ctx context.Context, q repo.AlbumAdminListQuery, offset, limit int) ([]entity.Album, int, error) {
	items, err := uc.albums.List(ctx, q, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.albums.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		url, _, perr := uc.resolveAlbumPreviewURL(ctx, items[i])
		if perr != nil {
			// Preview is best-effort; skip without failing the whole list.
			continue
		}
		items[i].PreviewURL = url
	}
	return items, total, nil
}

// resolveAlbumPreviewURL picks the album's cover image (if any) or the lowest-id
// image as a fallback, then resolves it to a public URL.
func (uc *UseCase) resolveAlbumPreviewURL(ctx context.Context, album entity.Album) (string, bool, error) {
	var img entity.Image
	if album.HasCover && album.CoverImageID > 0 {
		var err error
		img, err = uc.images.GetByID(ctx, album.CoverImageID)
		if err != nil {
			return "", false, err
		}
	} else {
		fallback, found, err := uc.images.GetFirstByAlbum(ctx, album.ID)
		if err != nil {
			return "", false, err
		}
		if !found {
			return "", false, nil
		}
		img = fallback
	}
	url, err := uc.imagesUC.ResolveURL(ctx, img)
	if err != nil {
		return "", false, err
	}
	return url, true, nil
}

func (uc *UseCase) GetAlbum(ctx context.Context, id int) (entity.Album, error) {
	return uc.albums.GetByID(ctx, id)
}

func normalizeAlbumSendMode(sendMode entity.AlbumSendMode) (entity.AlbumSendMode, error) {
	mode := entity.AlbumSendMode(strings.TrimSpace(string(sendMode)))
	switch mode {
	case "":
		return entity.AlbumSendModeRandom, nil
	case entity.AlbumSendModeOrder, entity.AlbumSendModeRandom, entity.AlbumSendModeSingle, entity.AlbumSendModeCustom:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid album send mode: %s", mode)
	}
}

func normalizeAlbumSendConfigJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", fmt.Errorf("send_config_json must be valid JSON: %w", err)
	}
	return raw, nil
}

func (uc *UseCase) CreateAlbum(ctx context.Context, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Album{}, fmt.Errorf("album name is required")
	}
	mode, err := normalizeAlbumSendMode(sendMode)
	if err != nil {
		return entity.Album{}, err
	}
	configJSON, err := normalizeAlbumSendConfigJSON(sendConfigJSON)
	if err != nil {
		return entity.Album{}, err
	}
	return uc.albums.Create(ctx, name, mode, configJSON)
}

func (uc *UseCase) UpdateAlbum(ctx context.Context, id int, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entity.Album{}, fmt.Errorf("album name is required")
	}
	mode, err := normalizeAlbumSendMode(sendMode)
	if err != nil {
		return entity.Album{}, err
	}
	configJSON, err := normalizeAlbumSendConfigJSON(sendConfigJSON)
	if err != nil {
		return entity.Album{}, err
	}
	return uc.albums.Update(ctx, id, name, mode, configJSON)
}

func (uc *UseCase) DeleteAlbum(ctx context.Context, id int) error {
	return uc.albums.Delete(ctx, id)
}

// ListImages returns paginated images with preview URLs already resolved.
// pCloud URLs go through PCloudClient's in-memory cache to limit upstream API calls.
func (uc *UseCase) ListImages(ctx context.Context, q repo.ImageAdminListQuery, offset, limit int) ([]entity.Image, int, error) {
	items, err := uc.images.List(ctx, q, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.images.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	for i := range items {
		url, perr := uc.imagesUC.ResolveURL(ctx, items[i])
		if perr != nil {
			continue
		}
		items[i].PreviewURL = url
	}
	return items, total, nil
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

func (uc *UseCase) SendAlbumTest(ctx context.Context, guildID string, albumID int, actor string) (entity.ManualScheduleTriggerResult, error) {
	if uc.runtime == nil {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("runtime trigger is not available")
	}
	res, err := uc.runtime.SendAlbumTest(ctx, guildID, albumID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "album.send_test", "album", strconv.Itoa(albumID), map[string]any{
		"guild_id":   guildID,
		"album_id":   res.AlbumID,
		"album_name": res.AlbumName,
		"channel_id": res.ChannelID,
		"message_id": res.MessageID,
	})
	return res, nil
}
