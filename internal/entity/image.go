// Package entity defines main entities for business logic.
package entity

// Image represents an image (URL and optional metadata).
type Image struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`      // pCloud path or local path
	Source    string `json:"source,omitempty"`
	GuildID   string `json:"guild_id,omitempty"`
	AlbumID   int    `json:"album_id,omitempty"`
	AlbumName string `json:"album_name,omitempty"`
	FileID    int64  `json:"file_id,omitempty"` // pCloud file ID for link generation
	IsCover   bool   `json:"is_cover,omitempty"` // set by use case when image is the album cover
}
