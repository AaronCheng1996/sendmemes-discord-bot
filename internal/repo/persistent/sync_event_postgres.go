package persistent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
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
		Columns("event_type", "album_id", "album_name", "new_images", "new_videos", "removed_images", "removed_videos", "previous_name", "file_names").
		Values(ev.EventType, nullableInt(ev.AlbumID), ev.AlbumName, ev.NewImages, ev.NewVideos, ev.RemovedImages, ev.RemovedVideos, nullableString(ev.PreviousName), rawNames).
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
		Select(
			"id", "event_type", "COALESCE(album_id, 0)", "album_name",
			"new_images", "new_videos", "removed_images", "removed_videos",
			"COALESCE(previous_name, '')", "file_names", "created_at",
		).
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
		if err = rows.Scan(
			&ev.ID, &ev.EventType, &ev.AlbumID, &ev.AlbumName,
			&ev.NewImages, &ev.NewVideos, &ev.RemovedImages, &ev.RemovedVideos,
			&ev.PreviousName, &rawNames, &ev.CreatedAt,
		); err != nil {
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

// GetByID returns one event by primary key.
func (r *SyncEventsRepo) GetByID(ctx context.Context, id int64) (entity.SyncEvent, error) {
	sql, args, err := r.Builder.
		Select(
			"id", "event_type", "COALESCE(album_id, 0)", "album_name",
			"new_images", "new_videos", "removed_images", "removed_videos",
			"COALESCE(previous_name, '')", "file_names", "created_at",
		).
		From("sync_events").
		Where(sq.Eq{"id": id}).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - GetByID - r.Builder: %w", err)
	}

	var ev entity.SyncEvent
	var rawNames []byte
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(
		&ev.ID, &ev.EventType, &ev.AlbumID, &ev.AlbumName,
		&ev.NewImages, &ev.NewVideos, &ev.RemovedImages, &ev.RemovedVideos,
		&ev.PreviousName, &rawNames, &ev.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - GetByID - event %d not found", id)
		}
		return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - GetByID - QueryRow: %w", err)
	}
	if len(rawNames) > 0 {
		if err = json.Unmarshal(rawNames, &ev.FileNames); err != nil {
			return entity.SyncEvent{}, fmt.Errorf("SyncEventsRepo - GetByID - Unmarshal file_names: %w", err)
		}
	}

	return ev, nil
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

// LatestAt returns the created_at of the most recent event, or nil when no
// sync event has ever been recorded.
func (r *SyncEventsRepo) LatestAt(ctx context.Context) (*time.Time, error) {
	sql, args, err := r.Builder.
		Select("created_at").
		From("sync_events").
		OrderBy("created_at DESC").
		Limit(1).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("SyncEventsRepo - LatestAt - r.Builder: %w", err)
	}
	var t time.Time
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("SyncEventsRepo - LatestAt - QueryRow: %w", err)
	}
	return &t, nil
}
