package entity

import "time"

// AdminAuditLog captures a privileged action for auditing.
type AdminAuditLog struct {
	ID         int64          `json:"id"`
	Actor      string         `json:"actor"`
	Action     string         `json:"action"`
	TargetType string         `json:"target_type"`
	TargetID   string         `json:"target_id"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

// SystemStatus summarizes runtime status for admin dashboard.
type SystemStatus struct {
	ServerTime       time.Time `json:"server_time"`
	DatabaseStatus   string    `json:"database_status"`
	DiscordConnected bool      `json:"discord_connected"`
	DiscordUser      string    `json:"discord_user,omitempty"`
	// SyncInterval is the effective pCloud sync cadence.
	SyncInterval string `json:"sync_interval"`
	// RuleCount is the number of configured delivery rules.
	RuleCount int `json:"rule_count"`
	// NextScheduledRun is the earliest next_run_at among enabled scheduled
	// rules, or nil when there are none.
	NextScheduledRun *time.Time `json:"next_scheduled_run,omitempty"`
	// LastSyncAt is the created_at of the most recent sync event, or nil when
	// no sync has ever run.
	LastSyncAt *time.Time `json:"last_sync_at,omitempty"`
	AlbumCount int        `json:"album_count"`
	ImageCount int        `json:"image_count"`
	VideoCount int        `json:"video_count"`
}

// ManualScheduleTriggerResult represents one immediate scheduled send run.
type ManualScheduleTriggerResult struct {
	Triggered bool   `json:"triggered"`
	AlbumID   int    `json:"album_id,omitempty"`
	AlbumName string `json:"album_name,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}
