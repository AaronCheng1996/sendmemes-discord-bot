package persistent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// SyncEventsRepo persists pCloud sync discovery events.
type SyncEventsRepo struct {
	*postgres.Postgres
}

// NewSyncEventsRepo creates a new sync events repository.
func NewSyncEventsRepo(pg *postgres.Postgres) *SyncEventsRepo {
	return &SyncEventsRepo{Postgres: pg}
}

// Insert stores one event and returns it with ID and CreatedAt filled in.
func (r *SyncEventsRepo) Insert(ctx context.Context, ev entity.SyncEvent) (entity.SyncEvent, error) {
	names := ev.FileNames
	if names == nil {
		names = []string{} // store [] instead of JSON null
	}
	rawNames, err := json.Marshal(names)
	if err != nil {
		return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - Insert - json.Marshal: %w", err)
	}

	sql, args, err := r.Builder.
		Insert("sync_events").
		Columns("event_type", "album_id", "album_name", "new_images", "new_videos", "file_names").
		Values(ev.EventType, nullableInt(ev.AlbumID), ev.AlbumName, ev.NewImages, ev.NewVideos, rawNames).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - Insert - r.Builder: %w", err)
	}

	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&ev.ID, &ev.CreatedAt); err != nil {
		return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - Insert - QueryRow: %w", err)
	}
	return ev, nil
}

// List returns events newest-first with offset/limit pagination.
func (r *SyncEventsRepo) List(ctx context.Context, offset, limit int) ([]entity.SyncEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	sql, args, err := r.Builder.
		Select("id", "event_type", "COALESCE(album_id, 0)", "album_name", "new_images", "new_videos", "file_names", "created_at").
		From("sync_events").
		OrderBy("created_at DESC, id DESC").
		Offset(uint64(offset)).
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("SyncEventsRepo - List - r.Builder: %w", err)
	}

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("SyncEventsRepo - List - Query: %w", err)
	}
	defer rows.Close()

	events := make([]entity.SyncEvent, 0, limit)
	for rows.Next() {
		var ev entity.SyncEvent
		var rawNames []byte
		if err = rows.Scan(&ev.ID, &ev.EventType, &ev.AlbumID, &ev.AlbumName, &ev.NewImages, &ev.NewVideos, &rawNames, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("SyncEventsRepo - List - Scan: %w", err)
		}
		if len(rawNames) > 0 {
			if err = json.Unmarshal(rawNames, &ev.FileNames); err != nil {
				return nil, fmt.Errorf("SyncEventsRepo - List - Unmarshal file_names: %w", err)
			}
		}
		events = append(events, ev)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("SyncEventsRepo - List - rows.Err: %w", err)
	}
	return events, nil
}

// Count returns the total number of stored events.
func (r *SyncEventsRepo) Count(ctx context.Context) (int, error) {
	sql, args, err := r.Builder.Select("COUNT(*)").From("sync_events").ToSql()
	if err != nil {
		return 0, fmt.Errorf("SyncEventsRepo - Count - r.Builder: %w", err)
	}
	var n int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("SyncEventsRepo - Count - QueryRow: %w", err)
	}
	return n, nil
}
