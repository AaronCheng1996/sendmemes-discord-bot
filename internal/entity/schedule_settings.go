package entity

import "time"

// DiscordScheduleSettings stores runtime schedule settings per guild.
type DiscordScheduleSettings struct {
	GuildID         string `json:"guild_id"`
	SendChannelID   string `json:"send_channel_id,omitempty"`
	SendInterval    string `json:"send_interval,omitempty"`
	SendHistorySize int    `json:"send_history_size,omitempty"`
	// NotifyChannelID is the channel for sync discovery notifications (empty = disabled).
	NotifyChannelID string    `json:"notify_channel_id,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// EffectiveScheduleSettings is the resolved schedule used by runtime.
type EffectiveScheduleSettings struct {
	GuildID               string        `json:"guild_id"`
	SendChannelID         string        `json:"send_channel_id"`
	SendInterval          string        `json:"send_interval"`
	SendIntervalDuration  time.Duration `json:"-"`
	SendHistorySize       int           `json:"send_history_size"`
	NotifyChannelID       string        `json:"notify_channel_id"`
	SourceSendChannelID   string        `json:"source_send_channel_id"`
	SourceSendInterval    string        `json:"source_send_interval"`
	SourceSendHistorySize string        `json:"source_send_history_size"`
	SourceNotifyChannelID string        `json:"source_notify_channel_id"`
}
