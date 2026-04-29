// Package usecase implements application business logic. Each logic group in own file.
package usecase

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
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
		// GetScheduledAlbumImages picks a random album while avoiding the excludeN most
		// recently sent albums; returns up to limit images (cover-first) and the album ID.
		// When all albums fall within the history window, it resets and picks fully at random.
		// Call MarkAlbumSent(albumID) after a successful scheduled send.
		GetScheduledAlbumImages(ctx context.Context, excludeN, limit int) (imgs []entity.Image, albumID int, err error)
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
		// For pCloud images it calls GetFileLink; for local paths it prepends HTTP_PUBLIC_URL.
		ResolveURL(ctx context.Context, img entity.Image) (string, error)
	}

	Sync interface {
		// SyncImages fetches the pCloud folder tree and reconciles it with the database.
		SyncImages(ctx context.Context) error
	}

	Settings interface {
		GetEffectiveSchedule(ctx context.Context, guildID string) (entity.EffectiveScheduleSettings, error)
		UpsertSchedule(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error)
	}

	AdminRuntime interface {
		TriggerScheduleNow(ctx context.Context, guildID string) (entity.ManualScheduleTriggerResult, error)
		GetDiscordStatus(ctx context.Context) (connected bool, user string)
	}

	Admin interface {
		// ListAlbums returns paginated albums with embedded preview_url already resolved
		// (cover image when present, otherwise the lowest-id image in the album).
		ListAlbums(ctx context.Context, offset, limit int) ([]entity.Album, int, error)
		GetAlbum(ctx context.Context, id int) (entity.Album, error)
		CreateAlbum(ctx context.Context, name string) (entity.Album, error)
		UpdateAlbum(ctx context.Context, id int, name string) (entity.Album, error)
		DeleteAlbum(ctx context.Context, id int) error
		// ListImages returns paginated images with embedded preview_url already resolved.
		ListImages(ctx context.Context, albumID, offset, limit int) ([]entity.Image, int, error)
		GetImage(ctx context.Context, id int) (entity.Image, error)
		CreateImage(ctx context.Context, img entity.Image) (entity.Image, error)
		UpdateImage(ctx context.Context, img entity.Image) (entity.Image, error)
		DeleteImage(ctx context.Context, id int) error
		GetEffectiveSchedule(ctx context.Context, guildID string) (entity.EffectiveScheduleSettings, error)
		UpsertSchedule(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error)
		RecordAudit(ctx context.Context, actor, action, targetType, targetID string, metadata map[string]any) error
		GetSystemStatus(ctx context.Context, guildID string) (entity.SystemStatus, error)
		TriggerScheduleNow(ctx context.Context, guildID, actor string) (entity.ManualScheduleTriggerResult, error)
	}
)
