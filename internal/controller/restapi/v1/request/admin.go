package request

import "github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"

type AlbumCreate struct {
	Name           string `json:"name" validate:"required"`
	SendMode       string `json:"send_mode"`
	SendConfigJSON string `json:"send_config_json"`
}

type AlbumUpdate struct {
	Name           string `json:"name" validate:"required"`
	SendMode       string `json:"send_mode"`
	SendConfigJSON string `json:"send_config_json"`
}

type ImageCreate struct {
	URL     string `json:"url" validate:"required"`
	Source  string `json:"source"`
	GuildID string `json:"guild_id"`
	AlbumID int    `json:"album_id"`
	FileID  int64  `json:"file_id"`
}

type ImageUpdate struct {
	URL     string `json:"url" validate:"required"`
	Source  string `json:"source"`
	GuildID string `json:"guild_id"`
	AlbumID int    `json:"album_id"`
	FileID  int64  `json:"file_id"`
}

// DeliveryRuleWrite is the create/update body for a delivery rule. Enabled is a
// pointer so an omitted value defaults to true on create instead of false.
type DeliveryRuleWrite struct {
	Name         string `json:"name"`
	GuildID      string `json:"guild_id"`
	TriggerType  string `json:"trigger_type" validate:"required"`
	ChannelID    string `json:"channel_id" validate:"required"`
	SendInterval string `json:"send_interval"`
	HistorySize  int    `json:"history_size"`
	Enabled      *bool  `json:"enabled"`
	// MessageStyle overrides presentation for this rule. Unset fields inherit
	// the app defaults rather than clearing them.
	MessageStyle entity.MessageStyle `json:"message_style"`
}

// SyncSettingsPut updates the global sync cadence.
type SyncSettingsPut struct {
	SyncInterval string `json:"sync_interval" validate:"required"`
}

// MessageDefaultsPut sets the app-wide message presentation defaults — the
// bottom layer that delivery rules and albums override.
type MessageDefaultsPut struct {
	MessageStyle entity.MessageStyle `json:"message_style"`
}

// RuleTest previews a delivery rule; album_id 0 picks a random album.
type RuleTest struct {
	AlbumID int `json:"album_id"`
}

// ScheduleTriggerNow sends a random album now; empty channel_id falls back to
// the first enabled scheduled rule.
type ScheduleTriggerNow struct {
	ChannelID string `json:"channel_id"`
}

// AlbumSendTest triggers a one-off preview send for the album in the URL path.
type AlbumSendTest struct {
	ChannelID string `json:"channel_id"`
}
