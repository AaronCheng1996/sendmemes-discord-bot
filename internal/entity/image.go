// Package entity defines main entities for business logic.
package entity

import "time"

// Media kind values for Image.Kind (also enforced by the images.kind DB CHECK).
const (
	MediaKindImage = "image"
	MediaKindVideo = "video"
)

// Image represents an image or video (URL and optional metadata).
type Image struct {
	ID        int    `json:"id"`
	URL       string `json:"url"` // pCloud path or local path
	Source    string `json:"source,omitempty"`
	GuildID   string `json:"guild_id,omitempty"`
	AlbumID   int    `json:"album_id,omitempty"`
	AlbumName string `json:"album_name,omitempty"`
	FileID    int64  `json:"file_id,omitempty"`  // pCloud file ID for link generation
	IsCover   bool   `json:"is_cover,omitempty"` // set by use case when image is the album cover
	// Kind is "image" or "video" (see MediaKind* constants).
	Kind string `json:"kind"`
	// SizeBytes is the file size in bytes when known (0 when unknown).
	SizeBytes int64 `json:"size_bytes,omitempty"`
	// PreviewURL is resolved on demand by the admin list endpoint and is not persisted.
	// pCloud links are temporary; the caller is expected to re-fetch when needed.
	PreviewURL string `json:"preview_url,omitempty"`
	// PublicLink is a permanent pCloud public share URL (from getfilepublink).
	// Unlike temporary download links it never expires and is not IP-bound, so it
	// is persisted once and reused. Empty until first resolved.
	PublicLink string `json:"public_link,omitempty"`
	// DeletedAt is set when a sync run no longer finds the file in its source.
	// The row is kept rather than dropped so the file is revived if it comes
	// back, and so the activity log can still name what disappeared. Every read
	// path skips deleted rows; the admin list opts back in explicitly.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
