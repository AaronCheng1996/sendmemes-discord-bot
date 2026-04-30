// Package entity defines main entities for business logic.
package entity

import "time"

type AlbumSendMode string

const (
	AlbumSendModeOrder  AlbumSendMode = "Order"
	AlbumSendModeRandom AlbumSendMode = "Random"
	AlbumSendModeSingle AlbumSendMode = "Single"
	AlbumSendModeCustom AlbumSendMode = "Custom"
)

// Album represents a named collection of images (derived from folder name).
type Album struct {
	ID             int           `json:"id"`
	Name           string        `json:"name"`
	HasCover       bool          `json:"has_cover"`
	CoverImageID   int           `json:"cover_image_id,omitempty"`
	SendMode       AlbumSendMode `json:"send_mode"`
	SendConfigJSON string        `json:"send_config_json,omitempty"`
	LastSentAt     *time.Time    `json:"last_sent_at,omitempty"`
	PositiveRating int           `json:"positive_rating"`
	// PreviewURL is resolved on demand by the admin list endpoint (cover image
	// when present, otherwise the lowest-id image in the album). Not persisted.
	PreviewURL string `json:"preview_url,omitempty"`
}
