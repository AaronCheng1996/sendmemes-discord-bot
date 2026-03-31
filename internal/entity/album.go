// Package entity defines main entities for business logic.
package entity

import "time"

// Album represents a named collection of images (derived from folder name).
type Album struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	HasCover       bool       `json:"has_cover"`
	CoverImageID   int        `json:"cover_image_id,omitempty"`
	LastSentAt     *time.Time `json:"last_sent_at,omitempty"`
	PositiveRating int        `json:"positive_rating"`
}
