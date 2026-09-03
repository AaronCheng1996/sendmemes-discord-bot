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
	taskRuns    usecase.TaskRuns
	system      repo.SystemRepo
	runtime     usecase.AdminRuntime
	jobs        usecase.Jobs
	// defaultSendMode is applied when CreateAlbum/UpdateAlbum omit a send mode.
	defaultSendMode entity.AlbumSendMode
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
	taskRuns usecase.TaskRuns,
	system repo.SystemRepo,
	runtime usecase.AdminRuntime,
	jobs usecase.Jobs,
	defaultSendMode entity.AlbumSendMode,
) *UseCase {
	return &UseCase{
		albums:          albums,
		images:          images,
		imagesUC:        imagesUC,
		rules:           rules,
		appSettings:     appSettings,
		audit:           audit,
		syncEvents:      syncEvents,
		taskRuns:        taskRuns,
		system:          system,
		runtime:         runtime,
		jobs:            jobs,
		defaultSendMode: defaultSendMode,
	}
}

// ListAlbums returns paginated albums with preview URLs already resolved.
// Preview rule: cover image (if has_cover && cover_image_id) → first image in album → empty.
// pCloud previews are getpubthumb thumbnails off the persisted public share
// link, so repeat listings cost no upstream API calls once resolved.
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
	url, err := uc.imagesUC.ResolvePreviewURL(ctx, img)
	if err != nil {
		return "", false, err
	}
	return url, true, nil
}

func (uc *UseCase) GetAlbum(ctx context.Context, id int) (entity.Album, error) {
	return uc.albums.GetByID(ctx, id)
}

// normalizeAlbumSendMode validates sendMode, substituting the configured
// default when it is empty (instead of the hardcoded Random from ParseAlbumSendMode).
func (uc *UseCase) normalizeAlbumSendMode(sendMode entity.AlbumSendMode) (entity.AlbumSendMode, error) {
	if strings.TrimSpace(string(sendMode)) == "" {
		return uc.defaultSendMode, nil
	}
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
	mode, err := uc.normalizeAlbumSendMode(sendMode)
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
	mode, err := uc.normalizeAlbumSendMode(sendMode)
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

// TestRule queues a preview of one delivery rule, so operators can see the
// styling a rule produces without waiting for its trigger.
func (uc *UseCase) TestRule(ctx context.Context, ruleID int64, albumID int, actor string) (entity.Job, error) {
	if uc.runtime == nil {
		return entity.Job{}, fmt.Errorf("runtime trigger is not available")
	}
	rule, err := uc.rules.Get(ctx, ruleID)
	if err != nil {
		return entity.Job{}, err
	}

	label := rule.Name
	if label == "" {
		label = fmt.Sprintf("rule #%d", rule.ID)
	}

	return uc.jobs.Start(entity.JobKindSendTest, label, func(jobCtx context.Context) (map[string]any, error) {
		res, rerr := uc.runtime.SendRuleTest(jobCtx, ruleID, albumID)
		if rerr != nil {
			return nil, rerr
		}
		_ = uc.RecordAudit(jobCtx, actor, "rule.test", "delivery_rule", strconv.FormatInt(ruleID, 10), map[string]any{
			"album_id":   res.AlbumID,
			"album_name": res.AlbumName,
			"channel_id": res.ChannelID,
			"message_id": res.MessageID,
		})
		return map[string]any{"album": res.AlbumName, "channel_id": res.ChannelID}, nil
	}), nil
}

// UpdateMessageDefaults stores the app-wide message presentation defaults.
func (uc *UseCase) UpdateMessageDefaults(ctx context.Context, style entity.MessageStyle, actor string) (entity.AppSettings, error) {
	out, err := uc.appSettings.SetMessageDefaults(ctx, style)
	if err != nil {
		return entity.AppSettings{}, err
	}
	_ = uc.RecordAudit(ctx, actor, "settings.message_defaults", "app_settings", "message_defaults", map[string]any{
		"use_embed": style.UseEmbed,
		"title":     style.Title,
		"body":      style.Body,
	})
	return out, nil
}

func (uc *UseCase) TriggerSyncNow(ctx context.Context, actor string) (entity.Job, error) {
	if uc.runtime == nil {
		return entity.Job{}, fmt.Errorf("runtime trigger is not available")
	}
	job := uc.jobs.Start(entity.JobKindSync, "pCloud sync", func(jctx context.Context) (map[string]any, error) {
		report, err := uc.runtime.TriggerSyncNow(jctx)
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"events":         len(report.Events),
			"initial_import": report.InitialImport,
		}
		_ = uc.RecordAudit(jctx, actor, "sync.trigger_now", "sync", "", result)
		return result, nil
	})
	return job, nil
}

// ListJobs returns the most recent background jobs, newest first.
func (uc *UseCase) ListJobs(_ context.Context) []entity.Job {
	if uc.jobs == nil {
		return nil
	}
	return uc.jobs.List()
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

// ListSyncEventMedia resolves an activity event's sampled file names to image
// rows with preview URLs.
//
// The event stores at most a handful of names, which is the cap on what this can
// return — and comfortably more than the dashboard shows. Matching on names
// rather than "the album's newest files" is what keeps an old event showing what
// *it* was about rather than whatever arrived since.
func (uc *UseCase) ListSyncEventMedia(ctx context.Context, eventID int64) ([]entity.Image, error) {
	ev, err := uc.syncEvents.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if ev.AlbumID == 0 || len(ev.FileNames) == 0 {
		return nil, nil
	}

	items, err := uc.images.ListByAlbumAndNames(ctx, ev.AlbumID, ev.FileNames)
	if err != nil {
		return nil, err
	}
	uc.attachPreviewURLs(ctx, items)

	return items, nil
}

// ListAlbumMedia returns the album's first `limit` live files for the dashboard's
// expanded row, cover first. The cover is hoisted here rather than in the
// browser because the server is what knows which image it is.
func (uc *UseCase) ListAlbumMedia(ctx context.Context, albumID, limit int) ([]entity.Image, error) {
	if limit <= 0 {
		limit = 6
	}
	album, err := uc.albums.GetByID(ctx, albumID)
	if err != nil {
		return nil, err
	}

	items, err := uc.images.List(ctx, repo.ImageAdminListQuery{
		AlbumScopeID: albumID,
		SortBy:       "id",
		SortAsc:      true,
	}, 0, limit)
	if err != nil {
		return nil, err
	}
	if album.HasCover && album.CoverImageID > 0 {
		hoistCover(items, album.CoverImageID)
	}
	uc.attachPreviewURLs(ctx, items)

	return items, nil
}

// hoistCover moves the cover to the front of items, in place.
func hoistCover(items []entity.Image, coverID int) {
	for i := range items {
		if items[i].ID != coverID {
			continue
		}
		cover := items[i]
		copy(items[1:i+1], items[:i])
		items[0] = cover

		return
	}
}

// attachPreviewURLs fills in PreviewURL for a set of images.
//
// It resolves *preview* URLs, not download ones: a pCloud download link is
// minted for the server and is both short-lived and IP-bound, so a browser on
// any other address gets nothing. The preview link is the permanent public
// thumbnail — the same one the album cover has always used.
//
// Best-effort per image: one that cannot be resolved still belongs in the
// response, where the caller renders its filename instead.
func (uc *UseCase) attachPreviewURLs(ctx context.Context, items []entity.Image) {
	for i := range items {
		url, err := uc.imagesUC.ResolvePreviewURL(ctx, items[i])
		if err != nil {
			continue
		}
		items[i].PreviewURL = url
	}
}

// ListTaskRuns returns a page of the durable run log plus its total.
func (uc *UseCase) ListTaskRuns(ctx context.Context, q repo.TaskRunListQuery, offset, limit int) ([]entity.TaskRun, int, error) {
	return uc.taskRuns.List(ctx, q, offset, limit)
}

// ListTaskRunSources returns the distinct sources that have reported runs.
func (uc *UseCase) ListTaskRunSources(ctx context.Context) ([]string, error) {
	return uc.taskRuns.Sources(ctx)
}

// HasIngestAPIKey reports whether the run-reporting endpoint has a credential
// in force. Deliberately a boolean: an admin may replace the key, not read it.
func (uc *UseCase) HasIngestAPIKey(ctx context.Context) (bool, error) {
	return uc.appSettings.HasIngestAPIKey(ctx)
}

// SetIngestAPIKey replaces the run-reporting credential and returns whether one
// is now in force. The audit entry records that it changed, never the value.
func (uc *UseCase) SetIngestAPIKey(ctx context.Context, key, actor string) (bool, error) {
	if err := uc.appSettings.SetIngestAPIKey(ctx, key); err != nil {
		return false, err
	}
	configured, err := uc.appSettings.HasIngestAPIKey(ctx)
	if err != nil {
		return false, err
	}
	_ = uc.RecordAudit(ctx, actor, "settings.ingest_key", "settings", "ingest_api_key", map[string]any{
		"configured": configured,
	})

	return configured, nil
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

	nextRun, err := uc.nextScheduledRun(ctx)
	if err != nil {
		return entity.SystemStatus{}, err
	}
	lastSync, err := uc.syncEvents.LatestAt(ctx)
	if err != nil {
		return entity.SystemStatus{}, err
	}
	albumCount, err := uc.albums.Count(ctx, repo.AlbumAdminListQuery{})
	if err != nil {
		return entity.SystemStatus{}, err
	}
	imageCount, err := uc.images.CountByKind(ctx, entity.MediaKindImage)
	if err != nil {
		return entity.SystemStatus{}, err
	}
	videoCount, err := uc.images.CountByKind(ctx, entity.MediaKindVideo)
	if err != nil {
		return entity.SystemStatus{}, err
	}

	return entity.SystemStatus{
		ServerTime:       time.Now().UTC(),
		DatabaseStatus:   dbStatus,
		DiscordConnected: connected,
		DiscordUser:      user,
		SyncInterval:     interval,
		RuleCount:        ruleCount,
		NextScheduledRun: nextRun,
		LastSyncAt:       lastSync,
		AlbumCount:       albumCount,
		ImageCount:       imageCount,
		VideoCount:       videoCount,
	}, nil
}

// nextScheduledRun returns the earliest NextRunAt among enabled scheduled
// rules, or nil when there are none.
func (uc *UseCase) nextScheduledRun(ctx context.Context) (*time.Time, error) {
	scheduled, err := uc.rules.ListActiveByTrigger(ctx, entity.TriggerScheduled)
	if err != nil {
		return nil, err
	}
	var earliest *time.Time
	for _, rule := range scheduled {
		if rule.NextRunAt == nil {
			continue
		}
		if earliest == nil || rule.NextRunAt.Before(*earliest) {
			earliest = rule.NextRunAt
		}
	}
	return earliest, nil
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

func (uc *UseCase) TriggerScheduleNow(ctx context.Context, channelID, actor string) (entity.Job, error) {
	if uc.runtime == nil {
		return entity.Job{}, fmt.Errorf("runtime trigger is not available")
	}
	ch, history, err := uc.resolveTargetChannel(ctx, channelID)
	if err != nil {
		return entity.Job{}, err
	}
	job := uc.jobs.Start(entity.JobKindScheduleSend, ch, func(jctx context.Context) (map[string]any, error) {
		res, err := uc.runtime.TriggerScheduleNow(jctx, ch, history)
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"triggered":  res.Triggered,
			"album_id":   res.AlbumID,
			"album_name": res.AlbumName,
			"channel_id": res.ChannelID,
			"message_id": res.MessageID,
		}
		_ = uc.RecordAudit(jctx, actor, "schedule.trigger_now", "schedule", ch, result)
		return result, nil
	})
	return job, nil
}

func (uc *UseCase) SendAlbumTest(ctx context.Context, albumID int, channelID, actor string) (entity.Job, error) {
	if uc.runtime == nil {
		return entity.Job{}, fmt.Errorf("runtime trigger is not available")
	}
	ch, _, err := uc.resolveTargetChannel(ctx, channelID)
	if err != nil {
		return entity.Job{}, err
	}
	// Resolve the album name up front for a friendlier job label; fall back to
	// the id when the album cannot be loaded (the job itself will surface the error).
	label := strconv.Itoa(albumID)
	if album, aerr := uc.albums.GetByID(ctx, albumID); aerr == nil && strings.TrimSpace(album.Name) != "" {
		label = album.Name
	}
	job := uc.jobs.Start(entity.JobKindSendTest, label, func(jctx context.Context) (map[string]any, error) {
		res, err := uc.runtime.SendAlbumTest(jctx, ch, albumID)
		if err != nil {
			return nil, err
		}
		result := map[string]any{
			"channel_id": res.ChannelID,
			"album_id":   res.AlbumID,
			"album_name": res.AlbumName,
			"message_id": res.MessageID,
		}
		_ = uc.RecordAudit(jctx, actor, "album.send_test", "album", strconv.Itoa(albumID), result)
		return result, nil
	})
	return job, nil
}
