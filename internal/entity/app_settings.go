package entity

import "time"

// AppSettings holds global runtime settings (single row in app_settings).
type AppSettings struct {
	// SyncInterval is the pCloud sync cadence as a Go duration string (e.g. "1h").
	SyncInterval string `json:"sync_interval"`
	// MessageStyle is the bottom layer of message presentation; delivery rules
	// and albums override it per field.
	MessageStyle MessageStyle `json:"message_style"`
	// IngestAPIKey guards POST /v1/runs. It is deliberately json:"-": the API
	// reports only whether a key is set, never the key itself, so an admin
	// session cannot leak the credential through a response body.
	IngestAPIKey string    `json:"-"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// Style exposes the app-wide defaults as the bottom message-style layer.
func (s AppSettings) Style() MessageStyle { return s.MessageStyle }
