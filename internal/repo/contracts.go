// Package repo implements application outer layer logic. Each logic group in own file.
package repo

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

//go:generate mockgen -source=contracts.go -destination=../usecase/mocks_repo_test.go -package=usecase_test

// PCloudEntry is a single media file (image or video) discovered in the pCloud folder tree.
type PCloudEntry struct {
	FileID           int64
	Name             string
	ParentFolderName string // immediate parent folder name (= album name)
	Kind             string // "image" or "video" (see entity.MediaKind*)
	Size             int64  // file size in bytes (0 when unknown)
}

type (
	// TranslationRepo -.
	TranslationRepo interface {
		Store(context.Context, entity.Translation) error
		GetHistory(context.Context) ([]entity.Translation, error)
	}

	// TranslationWebAPI -.
	TranslationWebAPI interface {
		Translate(entity.Translation) (entity.Translation, error)
	}

	// AlbumsRepo manages album persistence.
	AlbumsRepo interface {
		List(ctx context.Context, q AlbumAdminListQuery, offset, limit int) ([]entity.Album, error)
		Count(ctx context.Context, q AlbumAdminListQuery) (int, error)
		GetByID(ctx context.Context, id int) (entity.Album, error)
		Create(ctx context.Context, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error)
		// GetOrCreate returns the album with the given name, creating it when
		// missing. The bool reports whether a new row was created.
		GetOrCreate(ctx context.Context, name string) (entity.Album, bool, error)
		GetByName(ctx context.Context, name string) (entity.Album, error)
		GetRandom(ctx context.Context) (entity.Album, error)
		Update(ctx context.Context, id int, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error)
		Delete(ctx context.Context, id int) error
		// GetRandomExcludeRecent returns a random album that is NOT among the
		// excludeN most recently sent (by last_sent_at DESC).
		// When no eligible album exists (all sent within the history window),
		// it falls back to GetRandom so the scheduler never stalls.
		GetRandomExcludeRecent(ctx context.Context, excludeN int) (entity.Album, error)
		// MarkSent stamps last_sent_at = NOW() for albumID.
		MarkSent(ctx context.Context, albumID int) error
		// IncrRating increments positive_rating by 1 for albumID.
		IncrRating(ctx context.Context, albumID int) error
		// SetCover marks an album as having a cover and records which image it is.
		SetCover(ctx context.Context, albumID, coverImageID int) error
		// ClearCover removes cover designation from an album.
		ClearCover(ctx context.Context, albumID int) error
	}

	// ImagesRepo manages image persistence.
	ImagesRepo interface {
		List(ctx context.Context, q ImageAdminListQuery, offset, limit int) ([]entity.Image, error)
		Count(ctx context.Context, q ImageAdminListQuery) (int, error)
		GetFirstByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error)
		GetByID(ctx context.Context, id int) (entity.Image, error)
		GetDefault(ctx context.Context) (entity.Image, error)
		GetRandom(ctx context.Context) (entity.Image, error)
		Insert(ctx context.Context, img entity.Image) (entity.Image, error)
		Update(ctx context.Context, img entity.Image) (entity.Image, error)
		Delete(ctx context.Context, id int) error
		// GetRandomByAlbum returns up to limit random images from albumID,
		// optionally excluding the image with excludeID (pass 0 for no exclusion).
		GetRandomByAlbum(ctx context.Context, albumID, limit, excludeID int) ([]entity.Image, error)
		// GetAllByAlbum returns all images in albumID ordered by id,
		// optionally excluding the image with excludeID (pass 0 for no exclusion).
		GetAllByAlbum(ctx context.Context, albumID, excludeID int) ([]entity.Image, error)
		// GetRandomVideoByAlbum returns one random video (kind='video') from albumID.
		// Returns (zero, false, nil) when the album has no videos.
		GetRandomVideoByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error)
		// UpsertByFileID inserts or updates an image record keyed on file_id.
		// The bool reports whether a new row was inserted (vs. updated).
		UpsertByFileID(ctx context.Context, img entity.Image) (bool, error)
		DeleteByAlbumNotInFileIDs(ctx context.Context, albumID int, fileIDs []int64) error
		// FindCoverByAlbum returns the image in albumID whose filename matches
		// the cover convention (cover.* or _cover.*), case-insensitive.
		FindCoverByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error)
		// SetPublicLink persists the permanent pCloud public share link for image id.
		SetPublicLink(ctx context.Context, id int, link string) error
	}

	// DeliveryRulesRepo manages configurable Discord delivery rules.
	DeliveryRulesRepo interface {
		List(ctx context.Context) ([]entity.DeliveryRule, error)
		// ListActiveByTrigger returns enabled rules of the given trigger type.
		ListActiveByTrigger(ctx context.Context, triggerType string) ([]entity.DeliveryRule, error)
		GetByID(ctx context.Context, id int64) (entity.DeliveryRule, error)
		Create(ctx context.Context, rule entity.DeliveryRule) (entity.DeliveryRule, error)
		Update(ctx context.Context, rule entity.DeliveryRule) (entity.DeliveryRule, error)
		Delete(ctx context.Context, id int64) error
		Count(ctx context.Context) (int, error)
	}

	// AppSettingsRepo persists global runtime settings (single row).
	AppSettingsRepo interface {
		Get(ctx context.Context) (entity.AppSettings, bool, error)
		Upsert(ctx context.Context, s entity.AppSettings) (entity.AppSettings, error)
	}

	// AdminAuditRepo stores admin action audit logs.
	AdminAuditRepo interface {
		Insert(ctx context.Context, log entity.AdminAuditLog) error
	}

	// SyncEventsRepo stores per-album discovery events from pCloud sync runs.
	SyncEventsRepo interface {
		// Insert stores one event and returns it with ID and CreatedAt filled in.
		Insert(ctx context.Context, ev entity.SyncEvent) (entity.SyncEvent, error)
		// List returns events newest-first with offset/limit pagination.
		List(ctx context.Context, offset, limit int) ([]entity.SyncEvent, error)
		Count(ctx context.Context) (int, error)
	}

	// SystemRepo provides system-level checks.
	SystemRepo interface {
		Ping(ctx context.Context) error
	}

	// PCloudAPI abstracts the pCloud REST API.
	PCloudAPI interface {
		ListFolder(ctx context.Context, folderID int64) ([]PCloudEntry, error)
		GetFileLink(ctx context.Context, fileID int64) (string, error)
		// GetFilePublicLink returns a permanent, non-IP-bound public share URL
		// for a file (pCloud getfilepublink). The link never expires, so callers
		// persist it rather than regenerating per request.
		GetFilePublicLink(ctx context.Context, fileID int64) (string, error)
	}
)
