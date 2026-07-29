package entity

import "time"

// AppSettings holds global runtime settings (single row in app_settings).
type AppSettings struct {
	// SyncInterval is the pCloud sync cadence as a Go duration string (e.g. "1h").
	SyncInterval string `json:"sync_interval"`
	// DefaultUseEmbed / DefaultTitle / DefaultBody are the bottom layer of
	// message presentation; delivery rules and albums override them.
	DefaultUseEmbed *bool     `json:"default_use_embed,omitempty"`
	DefaultTitle    string    `json:"default_title,omitempty"`
	DefaultBody     string    `json:"default_body,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// Style exposes the app-wide defaults as the bottom message-style layer.
func (s AppSettings) Style() MessageStyle {
	return MessageStyle{UseEmbed: s.DefaultUseEmbed, Title: s.DefaultTitle, Body: s.DefaultBody}
}
