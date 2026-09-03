package request

import (
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

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
	// AlbumFilter narrows the rule to part of the library by folder path. Omit
	// it, or send an empty object, to cover every album.
	AlbumFilter entity.AlbumPathFilter `json:"album_filter"`
}

// SyncSettingsPut updates the global sync cadence.
type SyncSettingsPut struct {
	SyncInterval string `json:"sync_interval" validate:"required"`
}

// IngestKeyPut replaces the credential guarding POST /v1/runs. An empty key
// clears the stored one, falling back to the INGEST_API_KEY env value.
type IngestKeyPut struct {
	IngestAPIKey string `json:"ingest_api_key"`
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

// TaskRunCreate reports one execution to the run log. A terminal status
// ("succeeded" / "failed") records a finished run in a single call; "running"
// opens a row for TaskRunComplete to close. Timestamps default to now.
type TaskRunCreate struct {
	Source     string         `json:"source" validate:"required"`
	Task       string         `json:"task"`
	Status     string         `json:"status" validate:"required"`
	StartedAt  *time.Time     `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at"`
	Summary    string         `json:"summary"`
	Detail     map[string]any `json:"detail"`
	Error      string         `json:"error"`
}

// TaskRunComplete closes a run that was opened with status "running".
type TaskRunComplete struct {
	Status     string         `json:"status" validate:"required"`
	FinishedAt *time.Time     `json:"finished_at"`
	Summary    string         `json:"summary"`
	Detail     map[string]any `json:"detail"`
	Error      string         `json:"error"`
}
