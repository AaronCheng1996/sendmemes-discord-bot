package entity

import "time"

// Sync event types recorded per album by a sync run. The first two report
// discovered content and can be delivered to Discord; the rest report what a run
// took away (or renamed) and exist for the admin activity log only — see
// SyncEventTriggerType.
const (
	SyncEventAlbumCreated = "album_created"
	SyncEventFilesAdded   = "files_added"
	// SyncEventAlbumRenamed records an album that followed its source folder to
	// a new name (matched on folder id), keeping its rating and config.
	SyncEventAlbumRenamed = "album_renamed"
	// SyncEventAlbumMissing records an album whose source folder disappeared.
	// Neither the album nor its files are dropped, only flagged.
	SyncEventAlbumMissing = "album_missing"
	// SyncEventFilesRemoved records files that vanished from a folder that is
	// itself still there.
	SyncEventFilesRemoved = "files_removed"
)

// SyncEvent records what one sync run changed in one album: content discovered,
// content removed, or a rename.
type SyncEvent struct {
	ID        int64  `json:"id"`
	EventType string `json:"event_type"` // see the SyncEvent* constants
	AlbumID   int    `json:"album_id,omitempty"`
	AlbumName string `json:"album_name"`
	NewImages int    `json:"new_images"`
	NewVideos int    `json:"new_videos"`
	// RemovedImages and RemovedVideos count the files this run soft-deleted
	// because the source no longer lists them.
	RemovedImages int `json:"removed_images"`
	RemovedVideos int `json:"removed_videos"`
	// PreviousName is the album's former name on a rename event.
	PreviousName string `json:"previous_name,omitempty"`
	// FileNames is a sample of the file names this event is about: discovered
	// ones for an add event, removed ones for a removal (capped, not exhaustive).
	FileNames []string  `json:"file_names,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// AlbumPath is the album's source path, carried in-memory so the notifier can
	// match a delivery rule's path filter without a second lookup. Never
	// persisted or serialized — the album row is the record of it.
	AlbumPath string `json:"-"`
	// NewMedia carries the newly discovered image/video records for this event.
	// It is populated in-memory for the Discord notifier only — never persisted
	// or serialized (the API surfaces counts and sampled names instead).
	NewMedia []Image `json:"-"`
}

// SyncReport summarizes one sync run for callers (e.g. the Discord notifier).
type SyncReport struct {
	// Events holds one entry per album that gained new content, ordered by album name.
	Events []SyncEvent `json:"events"`
	// InitialImport is true when the database had no albums before this run;
	// callers should suppress notifications to avoid flooding on first import.
	InitialImport bool `json:"initial_import"`
	// Notices holds the events this run recorded for the admin activity log
	// only — removals and renames. They are deliberately kept out of Events so
	// the Discord notifier cannot post an album's files back at the very moment
	// they disappeared.
	Notices []SyncEvent `json:"notices,omitempty"`
	// MissingAlbums names the albums this run flagged as missing because their
	// source folder disappeared. They keep their rating and config but are
	// skipped by scheduled delivery until the folder comes back.
	MissingAlbums []string `json:"missing_albums,omitempty"`
	// EmptyScan is true when the source returned no media at all. The missing
	// pass is skipped in that case, since it almost always means a broken
	// source rather than an intentionally emptied library.
	EmptyScan bool `json:"empty_scan,omitempty"`
}
