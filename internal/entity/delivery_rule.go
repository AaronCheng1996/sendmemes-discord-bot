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
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	GuildID      string    `json:"guild_id"`
	TriggerType  string    `json:"trigger_type"`
	ChannelID    string    `json:"channel_id"`
	SendInterval string    `json:"send_interval,omitempty"` // scheduled only
	HistorySize  int       `json:"history_size"`            // scheduled only
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
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
// should fire for it.
func SyncEventTriggerType(eventType string) string {
	if eventType == SyncEventAlbumCreated {
		return TriggerNewAlbum
	}
	return TriggerNewFiles
}
