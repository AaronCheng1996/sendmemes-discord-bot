// Package entity defines main entities for business logic.
package entity

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AlbumSendMode string

const (
	AlbumSendModeOrder  AlbumSendMode = "Order"
	AlbumSendModeRandom AlbumSendMode = "Random"
	AlbumSendModeSingle AlbumSendMode = "Single"
	AlbumSendModeVideo  AlbumSendMode = "Video"
	AlbumSendModeCustom AlbumSendMode = "Custom"
)

// ParseAlbumSendMode normalizes and validates a send-mode string.
// It trims surrounding whitespace; an empty string defaults to Random.
// Accepts Order, Random, Single, Video, and Custom; any other value is an error.
func ParseAlbumSendMode(s string) (AlbumSendMode, error) {
	mode := AlbumSendMode(strings.TrimSpace(s))
	switch mode {
	case "":
		return AlbumSendModeRandom, nil
	case AlbumSendModeOrder, AlbumSendModeRandom, AlbumSendModeSingle, AlbumSendModeVideo, AlbumSendModeCustom:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid album send mode: %s", mode)
	}
}

// AlbumSendConfig holds per-album delivery overrides parsed from
// Album.SendConfigJSON. All fields are optional; the zero value means "use the
// mode's default behavior". Custom mode reads every field; other modes honor
// BatchSize and Caption (see internal/controller/discord.deliverAlbum).
type AlbumSendConfig struct {
	BatchSize    int    `json:"batch_size,omitempty"`    // overrides the default per-message image count
	IncludeCover *bool  `json:"include_cover,omitempty"` // nil = default (include); false = exclude the cover
	Ordered      bool   `json:"ordered,omitempty"`       // true = natural filename order instead of random
	Caption      string `json:"caption,omitempty"`       // message body; overrides the rule's caption template
	NSFW         bool   `json:"nsfw,omitempty"`          // true = prefix attachment filenames with SPOILER_
	Title        string `json:"title,omitempty"`         // headline; overrides the rule's title template
	UseEmbed     *bool  `json:"use_embed,omitempty"`     // nil = inherit the rule/app preference
	// Embed-only overrides; see entity.MessageStyle for the full list.
	Color         string `json:"color,omitempty"`
	Footer        string `json:"footer,omitempty"`
	Author        string `json:"author,omitempty"`
	URL           string `json:"url,omitempty"`
	ShowImage     *bool  `json:"show_image,omitempty"`
	ShowThumbnail *bool  `json:"show_thumbnail,omitempty"`
	ShowTimestamp *bool  `json:"show_timestamp,omitempty"`
}

// Style exposes the album's presentation overrides as the top message-style
// layer, applied after the app defaults and the delivery rule.
func (c AlbumSendConfig) Style() MessageStyle {
	return MessageStyle{
		UseEmbed: c.UseEmbed, Title: c.Title, Body: c.Caption,
		Color: c.Color, Footer: c.Footer, Author: c.Author, URL: c.URL,
		ShowImage: c.ShowImage, ShowThumbnail: c.ShowThumbnail, ShowTimestamp: c.ShowTimestamp,
	}
}

// ParseAlbumSendConfig decodes raw (an album's stored send_config_json) into an
// AlbumSendConfig. An empty string or "{}" returns the zero value with no
// error — that is the common case for albums that have never set any config.
func ParseAlbumSendConfig(raw string) (AlbumSendConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return AlbumSendConfig{}, nil
	}
	var cfg AlbumSendConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return AlbumSendConfig{}, fmt.Errorf("invalid album send config: %w", err)
	}
	return cfg, nil
}

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
	// MissingSince is set when a sync run no longer finds the album's source
	// folder. Missing albums keep their rating and config but are excluded from
	// scheduled delivery; the field is cleared automatically if the folder
	// reappears.
	MissingSince *time.Time `json:"missing_since,omitempty"`
	// PreviewURL is resolved on demand by the admin list endpoint (cover image
	// when present, otherwise the lowest-id image in the album). Not persisted.
	PreviewURL string `json:"preview_url,omitempty"`
}
