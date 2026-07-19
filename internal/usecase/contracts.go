// Package usecase implements application business logic. Each logic group in own file.
package usecase

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

//go:generate mockgen -source=contracts.go -destination=./mocks_usecase_test.go -package=usecase_test

type (
	Translation interface {
		Translate(context.Context, entity.Translation) (entity.Translation, error)
		History(context.Context) (entity.TranslationHistory, error)
	}

	Images interface {
		// GetImage returns the default (fallback) image.
		GetImage(ctx context.Context) (entity.Image, error)
		// GetRandom returns one random image from all images.
		GetRandom(ctx context.Context) (entity.Image, error)
		// GetAlbumImages returns up to limit images from the named album.
		// If the album has a cover, it is always the first element; the rest are random.
		GetAlbumImages(ctx context.Context, albumName string, limit int) ([]entity.Image, error)
		// GetRandomAlbumImages picks a random album then returns up to limit images from it.
		// If the album has a cover, it is always the first element; the rest are random.
		GetRandomAlbumImages(ctx context.Context, limit int) ([]entity.Image, error)
		// GetScheduledAlbum picks a random album while avoiding the excludeN most
		// recently sent albums. When all albums fall within the history window it
		// resets and picks fully at random. Call MarkAlbumSent after a successful send.
		GetScheduledAlbum(ctx context.Context, excludeN int) (entity.Album, error)
		// GetAlbumByID returns the album with the given id.
		GetAlbumByID(ctx context.Context, id int) (entity.Album, error)
		// GetAlbumBatch returns up to limit images for album using cover-first rules:
		// cover first (when present), then random non-cover images.
		GetAlbumBatch(ctx context.Context, album entity.Album, limit int) ([]entity.Image, error)
		// GetComicPages returns the album's images in reading order: cover first (when
		// present), then remaining images sorted by natural filename order.
		GetComicPages(ctx context.Context, album entity.Album) ([]entity.Image, error)
		// GetRandomVideo returns one random video from the album.
		// Returns (image, true, nil) when a video exists, (zero, false, nil) when none.
		GetRandomVideo(ctx context.Context, albumID int) (entity.Image, bool, error)
		// SetAlbumMode updates the named album's send mode (preserving name and
		// send-config JSON) and returns the updated album.
		SetAlbumMode(ctx context.Context, albumName string, mode entity.AlbumSendMode) (entity.Album, error)
		// MarkAlbumSent stamps last_sent_at = NOW() for albumID so future scheduled sends
		// avoid it for the next excludeN rounds.
		MarkAlbumSent(ctx context.Context, albumID int) error
		// IncrAlbumRating increments positive_rating by 1 for albumID.
		// Called when a user adds any reaction to a scheduled-send message.
		IncrAlbumRating(ctx context.Context, albumID int) error
		// GetFullAlbum returns all non-cover images in the named album ordered by id.
		// The cover (if any) is sent separately by the caller via GetAlbumCover.
		GetFullAlbum(ctx context.Context, albumName string) ([]entity.Image, error)
		// GetAlbumCover returns the cover image for the named album.
		// Returns (image, true, nil) when a cover exists, (zero, false, nil) when it does not.
		GetAlbumCover(ctx context.Context, albumName string) (entity.Image, bool, error)
		// ResolveURL returns a public URL suitable for a Discord embed.
		// For pCloud images it calls GetFileLink; for local paths it prepends SENDMEMES_HTTP_PUBLIC_URL.
		ResolveURL(ctx context.Context, img entity.Image) (string, error)
		// ResolvePublicURL returns a permanent pCloud public share URL for img,
		// persisting it on first resolution. Unlike ResolveURL the link never
		// expires and is not IP-bound. Non-pCloud images fall back to ResolveURL.
		ResolvePublicURL(ctx context.Context, img entity.Image) (string, error)
	}

	Sync interface {
		// SyncImages fetches the pCloud folder tree and reconciles it with the
		// database, returning a report of newly discovered albums and files.
		SyncImages(ctx context.Context) (entity.SyncReport, error)
	}

	// Rules manages configurable Discord delivery rules.
	Rules interface {
		List(ctx context.Context) ([]entity.DeliveryRule, error)
		// ListActiveByTrigger returns enabled rules of the given trigger type.
		ListActiveByTrigger(ctx context.Context, triggerType string) ([]entity.DeliveryRule, error)
		Get(ctx context.Context, id int64) (entity.DeliveryRule, error)
		Create(ctx context.Context, rule entity.DeliveryRule) (entity.DeliveryRule, error)
		Update(ctx context.Context, id int64, rule entity.DeliveryRule) (entity.DeliveryRule, error)
		Delete(ctx context.Context, id int64) error
		Count(ctx context.Context) (int, error)
		// FirstScheduledChannel returns the channel and history size of the first
		// enabled scheduled rule, used as a default target for manual triggers.
		FirstScheduledChannel(ctx context.Context) (channelID string, historySize int, found bool, err error)
	}

	// AppSettings exposes global runtime settings.
	AppSettings interface {
		// GetSyncInterval returns the effective sync cadence (stored value or env default).
		GetSyncInterval(ctx context.Context) (string, error)
		SetSyncInterval(ctx context.Context, interval string) (entity.AppSettings, error)
	}

	AdminRuntime interface {
		// TriggerScheduleNow sends a random album immediately to channelID.
		TriggerScheduleNow(ctx context.Context, channelID string, historySize int) (entity.ManualScheduleTriggerResult, error)
		// SendAlbumTest posts a preview of albumID to channelID.
		// It does not update last_sent_at or anti-repeat history.
		SendAlbumTest(ctx context.Context, channelID string, albumID int) (entity.ManualScheduleTriggerResult, error)
		// TriggerSyncNow runs a pCloud sync immediately and posts notifications.
		TriggerSyncNow(ctx context.Context) (entity.SyncReport, error)
		GetDiscordStatus(ctx context.Context) (connected bool, user string)
	}

	Admin interface {
		// ListAlbums returns paginated albums with embedded preview_url already resolved
		// (cover image when present, otherwise the lowest-id image in the album).
		ListAlbums(ctx context.Context, q repo.AlbumAdminListQuery, offset, limit int) ([]entity.Album, int, error)
		GetAlbum(ctx context.Context, id int) (entity.Album, error)
		CreateAlbum(ctx context.Context, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error)
		UpdateAlbum(ctx context.Context, id int, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error)
		DeleteAlbum(ctx context.Context, id int) error
		// ListImages returns paginated images with embedded preview_url already resolved.
		ListImages(ctx context.Context, q repo.ImageAdminListQuery, offset, limit int) ([]entity.Image, int, error)
		GetImage(ctx context.Context, id int) (entity.Image, error)
		CreateImage(ctx context.Context, img entity.Image) (entity.Image, error)
		UpdateImage(ctx context.Context, img entity.Image) (entity.Image, error)
		DeleteImage(ctx context.Context, id int) error
		// Delivery rules CRUD.
		ListRules(ctx context.Context) ([]entity.DeliveryRule, error)
		GetRule(ctx context.Context, id int64) (entity.DeliveryRule, error)
		CreateRule(ctx context.Context, rule entity.DeliveryRule, actor string) (entity.DeliveryRule, error)
		UpdateRule(ctx context.Context, id int64, rule entity.DeliveryRule, actor string) (entity.DeliveryRule, error)
		DeleteRule(ctx context.Context, id int64, actor string) error
		// Sync settings + manual trigger.
		GetSyncSettings(ctx context.Context) (entity.AppSettings, error)
		UpdateSyncSettings(ctx context.Context, interval, actor string) (entity.AppSettings, error)
		TriggerSyncNow(ctx context.Context, actor string) (entity.SyncReport, error)
		RecordAudit(ctx context.Context, actor, action, targetType, targetID string, metadata map[string]any) error
		// ListSyncEvents returns paginated sync discovery events, newest first.
		ListSyncEvents(ctx context.Context, offset, limit int) ([]entity.SyncEvent, int, error)
		GetSystemStatus(ctx context.Context) (entity.SystemStatus, error)
		// TriggerScheduleNow sends a random album now to channelID (empty = first
		// enabled scheduled rule's channel).
		TriggerScheduleNow(ctx context.Context, channelID, actor string) (entity.ManualScheduleTriggerResult, error)
		// SendAlbumTest delivers a one-off preview for albumID to channelID (empty =
		// first enabled scheduled rule's channel).
		SendAlbumTest(ctx context.Context, albumID int, channelID, actor string) (entity.ManualScheduleTriggerResult, error)
	}
)
