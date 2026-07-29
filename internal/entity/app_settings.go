package entity

import "time"

// AppSettings holds global runtime settings (single row in app_settings).
type AppSettings struct {
	// SyncInterval is the pCloud sync cadence as a Go duration string (e.g. "1h").
	SyncInterval string `json:"sync_interval"`
	// MessageStyle is the bottom layer of message presentation; delivery rules
	// and albums override it per field.
	MessageStyle MessageStyle `json:"message_style"`
	UpdatedAt    time.Time    `json:"updated_at,omitempty"`
}

// Style exposes the app-wide defaults as the bottom message-style layer.
func (s AppSettings) Style() MessageStyle { return s.MessageStyle }
