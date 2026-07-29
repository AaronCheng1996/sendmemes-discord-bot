package entity

import (
	"encoding/json"
	"strings"
)

// MessageStyle is one layer of message presentation. Every field is optional:
// a nil pointer or an empty string means "inherit from the layer below".
//
// Layers are applied app defaults → delivery rule → album, so an album can
// override just its body while still inheriting the rule's title and the app's
// embed preference.
//
// It is persisted as JSONB (app_settings.message_style,
// delivery_rules.message_style, and inside albums.send_config_json), so new
// knobs can be added here without a migration.
type MessageStyle struct {
	// UseEmbed selects a rich embed (true) or a plain message (false).
	// nil inherits; the final fallback is DefaultUseEmbed.
	UseEmbed *bool `json:"use_embed,omitempty"`
	// Title is the headline. In embed mode it replaces the embed title (which
	// otherwise defaults to the album name); in plain mode it is sent as a bold
	// first line. Supports the same placeholders as Body.
	Title string `json:"title,omitempty"`
	// Body is the message text, previously called the caption.
	Body string `json:"body,omitempty"`

	// --- Embed-only options (ignored when UseEmbed resolves to false) ---

	// Color is the accent bar, as "#rrggbb". Empty uses the per-send-mode color.
	Color string `json:"color,omitempty"`
	// Footer replaces the default "album #12 · Random" footer.
	Footer string `json:"footer,omitempty"`
	// Author renders a small name above the title.
	Author string `json:"author,omitempty"`
	// URL makes the title a link.
	URL string `json:"url,omitempty"`
	// ShowImage renders the first attachment large inside the embed (default on).
	ShowImage *bool `json:"show_image,omitempty"`
	// ShowThumbnail renders the album's cover as a corner thumbnail (default on).
	ShowThumbnail *bool `json:"show_thumbnail,omitempty"`
	// ShowTimestamp stamps the embed with the send time (default off).
	ShowTimestamp *bool `json:"show_timestamp,omitempty"`
}

// DefaultUseEmbed is the built-in embed preference used when no layer sets one.
const DefaultUseEmbed = true

// ParseMessageStyle decodes a stored JSON style; empty input yields a zero
// style (i.e. "inherit everything").
func ParseMessageStyle(raw string) (MessageStyle, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return MessageStyle{}, nil
	}
	var style MessageStyle
	if err := json.Unmarshal([]byte(raw), &style); err != nil {
		return MessageStyle{}, err
	}
	return style, nil
}

// JSON encodes the style for persistence, always returning valid JSON.
func (s MessageStyle) JSON() string {
	raw, err := json.Marshal(s)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// mergeString keeps over when it has content, else falls back to base.
func mergeString(base, over string) string {
	if strings.TrimSpace(over) != "" {
		return over
	}
	return base
}

// mergeBool keeps over when set, else falls back to base.
func mergeBool(base, over *bool) *bool {
	if over != nil {
		v := *over
		return &v
	}
	return base
}

// MergeMessageStyle folds layers in precedence order — later layers win, but
// only for the fields they actually set, so a rule can override the title while
// leaving the app default's body intact.
func MergeMessageStyle(layers ...MessageStyle) MessageStyle {
	var out MessageStyle
	for _, layer := range layers {
		out.UseEmbed = mergeBool(out.UseEmbed, layer.UseEmbed)
		out.Title = mergeString(out.Title, layer.Title)
		out.Body = mergeString(out.Body, layer.Body)
		out.Color = mergeString(out.Color, layer.Color)
		out.Footer = mergeString(out.Footer, layer.Footer)
		out.Author = mergeString(out.Author, layer.Author)
		out.URL = mergeString(out.URL, layer.URL)
		out.ShowImage = mergeBool(out.ShowImage, layer.ShowImage)
		out.ShowThumbnail = mergeBool(out.ShowThumbnail, layer.ShowThumbnail)
		out.ShowTimestamp = mergeBool(out.ShowTimestamp, layer.ShowTimestamp)
	}
	return out
}

// EmbedEnabled resolves the merged embed preference against the built-in default.
func (s MessageStyle) EmbedEnabled() bool {
	if s.UseEmbed == nil {
		return DefaultUseEmbed
	}
	return *s.UseEmbed
}

// boolOr resolves an optional flag against a default.
func boolOr(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

// ImageEnabled reports whether the first attachment should render large.
func (s MessageStyle) ImageEnabled() bool { return boolOr(s.ShowImage, true) }

// ThumbnailEnabled reports whether the album cover thumbnail should show.
func (s MessageStyle) ThumbnailEnabled() bool { return boolOr(s.ShowThumbnail, true) }

// TimestampEnabled reports whether to stamp the embed with the send time.
func (s MessageStyle) TimestampEnabled() bool { return boolOr(s.ShowTimestamp, false) }
