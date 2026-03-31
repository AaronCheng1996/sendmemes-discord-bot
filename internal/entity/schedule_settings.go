package entity

import "time"

// DiscordScheduleSettings stores runtime schedule settings per guild.
type DiscordScheduleSettings struct {
	GuildID         string    `json:"guild_id"`
	SendChannelID   string    `json:"send_channel_id,omitempty"`
	SendInterval    string    `json:"send_interval,omitempty"`
	SendHistorySize int       `json:"send_history_size,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// EffectiveScheduleSettings is the resolved schedule used by runtime.
type EffectiveScheduleSettings struct {
	GuildID               string        `json:"guild_id"`
	SendChannelID         string        `json:"send_channel_id"`
	SendInterval          string        `json:"send_interval"`
	SendIntervalDuration  time.Duration `json:"-"`
	SendHistorySize       int           `json:"send_history_size"`
	SourceSendChannelID   string        `json:"source_send_channel_id"`
	SourceSendInterval    string        `json:"source_send_interval"`
	SourceSendHistorySize string        `json:"source_send_history_size"`
}
