package entity

import "time"

// Sync event types recorded per album when a pCloud sync run discovers new content.
const (
	SyncEventAlbumCreated = "album_created"
	SyncEventFilesAdded   = "files_added"
)

// SyncEvent records new content discovered in one album during a pCloud sync run.
type SyncEvent struct {
	ID        int64  `json:"id"`
	EventType string `json:"event_type"` // album_created | files_added
	AlbumID   int    `json:"album_id,omitempty"`
	AlbumName string `json:"album_name"`
	NewImages int    `json:"new_images"`
	NewVideos int    `json:"new_videos"`
	// FileNames is a sample of the newly discovered file names (capped, not exhaustive).
	FileNames []string  `json:"file_names,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
}
