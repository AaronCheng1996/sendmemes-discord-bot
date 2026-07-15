package entity

import "time"

// AppSettings holds global runtime settings (single row in app_settings).
type AppSettings struct {
	// SyncInterval is the pCloud sync cadence as a Go duration string (e.g. "1h").
	SyncInterval string    `json:"sync_interval"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}
