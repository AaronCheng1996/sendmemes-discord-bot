// Package entity defines main entities for business logic.
package entity

// Image represents an image (URL and optional metadata).
type Image struct {	
	ID       int    `json:"id"`
	URL      string `json:"url"`
	Source   string `json:"source,omitempty"`
	GuildID  string `json:"guild_id,omitempty"`	
}
