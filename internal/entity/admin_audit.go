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
	ServerTime       time.Time                 `json:"server_time"`
	DatabaseStatus   string                    `json:"database_status"`
	DiscordConnected bool                      `json:"discord_connected"`
	DiscordUser      string                    `json:"discord_user,omitempty"`
	EffectiveSchedule EffectiveScheduleSettings `json:"effective_schedule"`
}

// ManualScheduleTriggerResult represents one immediate scheduled send run.
type ManualScheduleTriggerResult struct {
	Triggered bool   `json:"triggered"`
	AlbumID   int    `json:"album_id,omitempty"`
	AlbumName string `json:"album_name,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}
