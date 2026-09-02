package entity

import (
	"fmt"
	"strings"
	"time"
)

// Delivery rule trigger types.
const (
	// TriggerNewAlbum fires when a pCloud sync creates a new album.
	TriggerNewAlbum = "new_album"
	// TriggerNewFiles fires when a pCloud sync adds files to an existing album.
	TriggerNewFiles = "new_files"
	// TriggerScheduled fires every SendInterval with a random album.
	TriggerScheduled = "scheduled"
)

// DeliveryRule is one configurable Discord delivery rule. Scheduled rules post a
// random album every SendInterval; new_album / new_files rules post freshly
// discovered media when a sync run reports it.
type DeliveryRule struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	GuildID      string `json:"guild_id"`
	TriggerType  string `json:"trigger_type"`
	ChannelID    string `json:"channel_id"`
	SendInterval string `json:"send_interval,omitempty"` // scheduled only
	HistorySize  int    `json:"history_size"`            // scheduled only
	Enabled      bool   `json:"enabled"`
	// MessageStyle overrides the app defaults for this rule's posts. Unset
	// fields inherit; the album's own config overrides whatever survives here.
	MessageStyle MessageStyle `json:"message_style"`
	CreatedAt    time.Time    `json:"created_at,omitempty"`
	UpdatedAt    time.Time    `json:"updated_at,omitempty"`

	// NextRunAt and ScheduleDescription are computed on read for scheduled
	// rules (never persisted) so the UI can show when a rule will next fire.
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`
	ScheduleDescription string     `json:"schedule_description,omitempty"`
}

// ParseTriggerType validates a trigger-type string.
func ParseTriggerType(s string) (string, error) {
	t := strings.TrimSpace(s)
	switch t {
	case TriggerNewAlbum, TriggerNewFiles, TriggerScheduled:
		return t, nil
	default:
		return "", fmt.Errorf("invalid trigger type: %q (want new_album, new_files, or scheduled)", s)
	}
}

// SyncEventTriggerType maps a sync event type to the delivery-rule trigger that
// should fire for it. Events that report a removal or a rename map to no
// trigger at all (empty string): they exist for the activity log, and posting an
// album's files to Discord because they just disappeared would be absurd.
func SyncEventTriggerType(eventType string) string {
	switch eventType {
	case SyncEventAlbumCreated:
		return TriggerNewAlbum
	case SyncEventFilesAdded:
		return TriggerNewFiles
	default:
		return ""
	}
}

// Style exposes the rule's presentation overrides as a message-style layer,
// sitting between the app defaults and the album's own config.
func (r DeliveryRule) Style() MessageStyle { return r.MessageStyle }
