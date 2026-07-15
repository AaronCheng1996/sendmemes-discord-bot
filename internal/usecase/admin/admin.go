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
	albums      repo.AlbumsRepo
	images      repo.ImagesRepo
	imagesUC    usecase.Images
	rules       usecase.Rules
	appSettings usecase.AppSettings
	audit       repo.AdminAuditRepo
	syncEvents  repo.SyncEventsRepo
	system      repo.SystemRepo
	runtime     usecase.AdminRuntime
}

// New creates admin usecase.
func New(
	albums repo.AlbumsRepo,
	images repo.ImagesRepo,
	imagesUC usecase.Images,
	rules usecase.Rules,
	appSettings usecase.AppSettings,
	audit repo.AdminAuditRepo,
	syncEvents repo.SyncEventsRepo,
	system repo.SystemRepo,
	runtime usecase.AdminRuntime,
) *UseCase {
	return &UseCase{
		albums:      albums,
		images:      images,
		imagesUC:    imagesUC,
		rules:       rules,
		appSettings: appSettings,
		audit:       audit,
		syncEvents:  syncEvents,
		system:      system,
		runtime:     runtime,
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
	return entity.ParseAlbumSendMode(string(sendMode))
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

// --- Delivery rules -------------------------------------------------------

func (uc *UseCase) ListRules(ctx context.Context) ([]entity.DeliveryRule, error) {
	return uc.rules.List(ctx)
}

func (uc *UseCase) GetRule(ctx context.Context, id int64) (entity.DeliveryRule, error) {
	return uc.rules.Get(ctx, id)
}

func (uc *UseCase) CreateRule(ctx context.Context, rule entity.DeliveryRule, actor string) (entity.DeliveryRule, error) {
	out, err := uc.rules.Create(ctx, rule)
	if err != nil {
		return entity.DeliveryRule{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "rule.create", "delivery_rule", strconv.FormatInt(out.ID, 10), map[string]any{
		"trigger_type": out.TriggerType, "channel_id": out.ChannelID, "name": out.Name,
	})
	return out, nil
}

func (uc *UseCase) UpdateRule(ctx context.Context, id int64, rule entity.DeliveryRule, actor string) (entity.DeliveryRule, error) {
	out, err := uc.rules.Update(ctx, id, rule)
	if err != nil {
		return entity.DeliveryRule{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "rule.update", "delivery_rule", strconv.FormatInt(id, 10), map[string]any{
		"trigger_type": out.TriggerType, "channel_id": out.ChannelID, "enabled": out.Enabled,
	})
	return out, nil
}

func (uc *UseCase) DeleteRule(ctx context.Context, id int64, actor string) error {
	if err := uc.rules.Delete(ctx, id); err != nil {
		return err
	}
	_ = uc.RecordAudit(ctx, actor, "rule.delete", "delivery_rule", strconv.FormatInt(id, 10), nil)
	return nil
}

// --- Sync settings + manual trigger ---------------------------------------

func (uc *UseCase) GetSyncSettings(ctx context.Context) (entity.AppSettings, error) {
	interval, err := uc.appSettings.GetSyncInterval(ctx)
	if err != nil {
		return entity.AppSettings{}, err
	}
	return entity.AppSettings{SyncInterval: interval}, nil
}

func (uc *UseCase) UpdateSyncSettings(ctx context.Context, interval, actor string) (entity.AppSettings, error) {
	out, err := uc.appSettings.SetSyncInterval(ctx, interval)
	if err != nil {
		return entity.AppSettings{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "sync.settings_update", "app_settings", "sync_interval", map[string]any{
		"sync_interval": out.SyncInterval,
	})
	return out, nil
}

func (uc *UseCase) TriggerSyncNow(ctx context.Context, actor string) (entity.SyncReport, error) {
	if uc.runtime == nil {
		return entity.SyncReport{}, fmt.Errorf("runtime trigger is not available")
	}
	report, err := uc.runtime.TriggerSyncNow(ctx)
	if err != nil {
		return entity.SyncReport{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "sync.trigger_now", "sync", "", map[string]any{
		"events":         len(report.Events),
		"initial_import": report.InitialImport,
	})
	return report, nil
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

// ListSyncEvents returns paginated sync discovery events, newest first.
func (uc *UseCase) ListSyncEvents(ctx context.Context, offset, limit int) ([]entity.SyncEvent, int, error) {
	items, err := uc.syncEvents.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.syncEvents.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (uc *UseCase) GetSystemStatus(ctx context.Context) (entity.SystemStatus, error) {
	interval, err := uc.appSettings.GetSyncInterval(ctx)
	if err != nil {
		return entity.SystemStatus{}, err
	}
	ruleCount, err := uc.rules.Count(ctx)
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
		ServerTime:       time.Now().UTC(),
		DatabaseStatus:   dbStatus,
		DiscordConnected: connected,
		DiscordUser:      user,
		SyncInterval:     interval,
		RuleCount:        ruleCount,
	}, nil
}

// resolveTargetChannel returns channelID when non-empty, otherwise the first
// enabled scheduled rule's channel and history size.
func (uc *UseCase) resolveTargetChannel(ctx context.Context, channelID string) (string, int, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID != "" {
		return channelID, 0, nil
	}
	ch, history, found, err := uc.rules.FirstScheduledChannel(ctx)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "", 0, fmt.Errorf("no channel specified and no enabled scheduled rule to fall back to")
	}
	return ch, history, nil
}

func (uc *UseCase) TriggerScheduleNow(ctx context.Context, channelID, actor string) (entity.ManualScheduleTriggerResult, error) {
	if uc.runtime == nil {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("runtime trigger is not available")
	}
	ch, history, err := uc.resolveTargetChannel(ctx, channelID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	res, err := uc.runtime.TriggerScheduleNow(ctx, ch, history)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "schedule.trigger_now", "schedule", ch, map[string]any{
		"triggered":  res.Triggered,
		"album_id":   res.AlbumID,
		"album_name": res.AlbumName,
		"channel_id": res.ChannelID,
		"message_id": res.MessageID,
	})
	return res, nil
}

func (uc *UseCase) SendAlbumTest(ctx context.Context, albumID int, channelID, actor string) (entity.ManualScheduleTriggerResult, error) {
	if uc.runtime == nil {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("runtime trigger is not available")
	}
	ch, _, err := uc.resolveTargetChannel(ctx, channelID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	res, err := uc.runtime.SendAlbumTest(ctx, ch, albumID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "album.send_test", "album", strconv.Itoa(albumID), map[string]any{
		"channel_id": res.ChannelID,
		"album_id":   res.AlbumID,
		"album_name": res.AlbumName,
		"message_id": res.MessageID,
	})
	return res, nil
}
