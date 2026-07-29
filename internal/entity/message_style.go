package entity

import "strings"

// MessageStyle is one layer of message presentation. Every field is optional:
// a nil UseEmbed or an empty Title/Body means "inherit from the layer below".
//
// Layers are applied app defaults → delivery rule → album, so an album can
// override just its body while still inheriting the rule's title and the app's
// embed preference.
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
}

// DefaultUseEmbed is the built-in embed preference used when no layer sets one.
const DefaultUseEmbed = true

// MergeMessageStyle folds layers in precedence order — later layers win, but
// only for the fields they actually set, so a rule can override the title while
// leaving the app default's body intact.
func MergeMessageStyle(layers ...MessageStyle) MessageStyle {
	var out MessageStyle
	for _, layer := range layers {
		if layer.UseEmbed != nil {
			v := *layer.UseEmbed
			out.UseEmbed = &v
		}
		if strings.TrimSpace(layer.Title) != "" {
			out.Title = layer.Title
		}
		if strings.TrimSpace(layer.Body) != "" {
			out.Body = layer.Body
		}
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
