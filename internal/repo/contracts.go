// Package repo implements application outer layer logic. Each logic group in own file.
package repo

import (
	"context"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

//go:generate mockgen -source=contracts.go -destination=../usecase/mocks_repo_test.go -package=usecase_test

// MediaEntry is a single media file (image or video) discovered by a MediaSource.
type MediaEntry struct {
	FileID           int64
	Name             string
	ParentFolderName string // immediate parent folder name (= album name)
	ParentFolderID   int64  // source's own id for that folder; 0 when it has none
	ParentPath       string // that folder's path from the walked root, root name included
	Kind             string // "image" or "video" (see entity.MediaKind*)
	Size             int64  // file size in bytes (0 when unknown)
}

// DiscoveredFolder identifies one folder a sync run walked.
type DiscoveredFolder struct {
	// ID is the source's own folder identifier; 0 when the source has none.
	ID int64
	// Name is the leaf folder name, which is the album's name.
	Name string
	// Path is the folder's full path from the walked root, root name included.
	Path string
}

// AlbumResolution reports how ResolveByFolder matched a discovered folder to an
// album row, so the sync can record the right activity event.
type AlbumResolution struct {
	// Created is true when no album matched and a new row was inserted.
	Created bool
	// RenamedFrom holds the album's previous name when the folder was matched
	// on its id and the row was renamed to follow it. Empty otherwise.
	RenamedFrom string
}

type (
	// AlbumsRepo manages album persistence.
	AlbumsRepo interface {
		List(ctx context.Context, q AlbumAdminListQuery, offset, limit int) ([]entity.Album, error)
		Count(ctx context.Context, q AlbumAdminListQuery) (int, error)
		GetByID(ctx context.Context, id int) (entity.Album, error)
		Create(ctx context.Context, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error)
		// ResolveByFolder maps a folder discovered by a sync run to its album
		// row, creating one with defaultMode as its send_mode when nothing
		// matches. The folder NAME is the album's identity and wins first: a row
		// already called folder.Name is the album, and folder.ID is (re)bound to
		// it. Only when no row carries the name does folder.ID take over, and the
		// row holding it is renamed in place so its rating, send mode and config
		// follow the folder. A folder.ID of 0 means the source has no folder ids;
		// resolution then falls back to name-only, as it was before. The walked
		// path is recorded on the album either way, for the rule path filters.
		ResolveByFolder(ctx context.Context, folder DiscoveredFolder, defaultMode entity.AlbumSendMode) (entity.Album, AlbumResolution, error)
		GetByName(ctx context.Context, name string) (entity.Album, error)
		// GetRandom returns a random album whose source path is in filter's
		// scope. Pass the zero filter to draw from the whole library.
		GetRandom(ctx context.Context, filter entity.AlbumPathFilter) (entity.Album, error)
		Update(ctx context.Context, id int, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error)
		Delete(ctx context.Context, id int) error
		// GetRandomExcludeRecent returns a random album in filter's scope that is
		// NOT among the excludeN most recently sent (by last_sent_at DESC).
		// When no eligible album exists (all sent within the history window),
		// it falls back to GetRandom — still filtered — so the scheduler never
		// stalls and never escapes the rule's scope.
		GetRandomExcludeRecent(ctx context.Context, excludeN int, filter entity.AlbumPathFilter) (entity.Album, error)
		// MarkSent stamps last_sent_at = NOW() for albumID.
		MarkSent(ctx context.Context, albumID int) error
		// IncrRating increments positive_rating by 1 for albumID.
		IncrRating(ctx context.Context, albumID int) error
		// SetCover marks an album as having a cover and records which image it is.
		SetCover(ctx context.Context, albumID, coverImageID int) error
		// ClearCover removes cover designation from an album.
		ClearCover(ctx context.Context, albumID int) error
		// MarkMissingExcept flags albums whose name was not seen in the latest
		// sync (missing_since = NOW(), already-marked albums keep their original
		// timestamp) and returns the albums it newly marked (id and name only).
		// It errors on an empty slice rather than marking everything.
		MarkMissingExcept(ctx context.Context, seenNames []string) ([]entity.Album, error)
		// ClearMissing unflags albums that reappeared in the latest sync.
		ClearMissing(ctx context.Context, seenNames []string) error
		// TopRated returns up to limit albums ordered by positive_rating DESC.
		TopRated(ctx context.Context, limit int) ([]entity.Album, error)
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
		// GetRandomByAlbum returns up to limit random media rows from albumID,
		// optionally excluding the row with excludeID (pass 0 for no exclusion).
		// Videos are included: Discord plays an uploaded video inline, so outside
		// Video mode a clip is just another attachment.
		GetRandomByAlbum(ctx context.Context, albumID, limit, excludeID int) ([]entity.Image, error)
		// GetAllByAlbum returns all media in albumID ordered by id, videos
		// included, optionally excluding the row with excludeID (0 = no exclusion).
		GetAllByAlbum(ctx context.Context, albumID, excludeID int) ([]entity.Image, error)
		// GetRandomVideoByAlbum returns one random video (kind='video') from albumID.
		// Returns (zero, false, nil) when the album has no videos.
		GetRandomVideoByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error)
		// UpsertByFileID inserts or updates an image record keyed on file_id.
		// The bool reports whether a new row was inserted (vs. updated). A row
		// that had been soft-deleted is revived rather than re-inserted, so a
		// file that moves away and back is not announced as new content twice.
		UpsertByFileID(ctx context.Context, img entity.Image) (bool, error)
		// SoftDeleteByAlbumNotInFileIDs flags rows owned by source in albumID
		// whose file_id is not in fileIDs, scoping the prune to the syncing
		// source so a local sync never touches pCloud-sourced rows (or vice
		// versa). Already-deleted rows keep their original timestamp; the
		// returned images are the ones this call newly flagged.
		SoftDeleteByAlbumNotInFileIDs(ctx context.Context, albumID int, source string, fileIDs []int64) ([]entity.Image, error)
		// SoftDeleteByAlbum flags every live row owned by source in albumID.
		// Used when the album's whole folder disappeared, so the dashboard stops
		// listing files that cannot be fetched any more.
		SoftDeleteByAlbum(ctx context.Context, albumID int, source string) ([]entity.Image, error)
		// FindCoverByAlbum returns the image in albumID whose filename matches
		// the cover convention (cover.* or _cover.*), case-insensitive.
		FindCoverByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error)
		// SetPublicLink persists the permanent pCloud public share link for image id.
		SetPublicLink(ctx context.Context, id int, link string) error
		// CountByKind returns the number of images with the given kind
		// (entity.MediaKindImage or entity.MediaKindVideo).
		CountByKind(ctx context.Context, kind string) (int, error)
		// CountAlbumMedia returns how many images and videos albumID holds.
		CountAlbumMedia(ctx context.Context, albumID int) (images, videos int, err error)
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
		// LatestAt returns the created_at of the most recent event, or nil when
		// no sync event has ever been recorded.
		LatestAt(ctx context.Context) (*time.Time, error)
	}

	// TaskRunsRepo stores one durable row per execution of a scheduled send, a
	// sync, or anything an ingest client reports. Unlike the in-memory job
	// manager it survives restarts and accepts writes from outside the process.
	TaskRunsRepo interface {
		// Insert stores a run and returns it with ID and CreatedAt filled in.
		Insert(ctx context.Context, run entity.TaskRun) (entity.TaskRun, error)
		// Complete replaces a run's outcome (status, finish time, summary,
		// detail, error), leaving what identifies the run untouched.
		Complete(ctx context.Context, id int64, outcome entity.TaskRun) (entity.TaskRun, error)
		List(ctx context.Context, q TaskRunListQuery, offset, limit int) ([]entity.TaskRun, error)
		Count(ctx context.Context, q TaskRunListQuery) (int, error)
		// Sources returns the distinct source values present, so the dashboard
		// filter does not have to hardcode which clients report runs.
		Sources(ctx context.Context) ([]string, error)
		// PruneBefore hard-deletes runs that started before cutoff, returning
		// how many went.
		PruneBefore(ctx context.Context, cutoff time.Time) (int64, error)
	}

	// SystemRepo provides system-level checks.
	SystemRepo interface {
		Ping(ctx context.Context) error
	}

	// MediaSource abstracts a source of media files (pCloud, local filesystem, ...).
	// Which roots to walk is the implementation's own concern (e.g. the pCloud
	// client holds its configured folder IDs) so callers never handle source-specific
	// identifiers.
	MediaSource interface {
		// ListMedia walks every configured root and returns all discovered media
		// files, each carrying its immediate parent folder name as the album
		// name, plus that folder's source id when the source has one.
		ListMedia(ctx context.Context) ([]MediaEntry, error)
		// ResolveDownloadURL returns a URL suitable for downloading m's file
		// content. May be temporary/IP-bound depending on the source.
		ResolveDownloadURL(ctx context.Context, m entity.Image) (string, error)
		// ResolveShareURL returns a permanent, non-IP-bound public share URL for
		// m. The link never expires, so callers persist it rather than
		// regenerating per request.
		ResolveShareURL(ctx context.Context, m entity.Image) (string, error)
		// ThumbURL turns a share URL from ResolveShareURL into a direct thumbnail
		// URL that renders in an <img> tag without auth. Pass an empty size for
		// the default geometry. Returns "" when shareURL carries no usable
		// thumbnail, leaving the fallback to the caller.
		ThumbURL(shareURL string, m entity.Image, size string) string
	}
)
