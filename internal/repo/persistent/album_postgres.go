package persistent

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
)

// AlbumsRepo -.
type AlbumsRepo struct {
	*postgres.Postgres
}

// NewAlbumsRepo -.
func NewAlbumsRepo(pg *postgres.Postgres) *AlbumsRepo {
	return &AlbumsRepo{Postgres: pg}
}

// GetOrCreate returns the album with the given name, creating it if it does not exist.
func (r *AlbumsRepo) GetOrCreate(ctx context.Context, name string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Insert("albums").
		Columns("name").
		Values(name).
		Suffix("ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id, name, has_cover, COALESCE(cover_image_id, 0)").
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetOrCreate - r.Builder: %w", err)
	}

	var a entity.Album
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&a.ID, &a.Name, &a.HasCover, &a.CoverImageID); err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetOrCreate - QueryRow: %w", err)
	}
	return a, nil
}

// GetByName returns the album with the given name.
func (r *AlbumsRepo) GetByName(ctx context.Context, name string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Select("id", "name", "has_cover", "COALESCE(cover_image_id, 0)").
		From("albums").
		Where("name = ?", name).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByName - r.Builder: %w", err)
	}

	var a entity.Album
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&a.ID, &a.Name, &a.HasCover, &a.CoverImageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByName - album %q not found", name)
		}
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByName - QueryRow: %w", err)
	}
	return a, nil
}

// GetRandom returns a random album.
func (r *AlbumsRepo) GetRandom(ctx context.Context) (entity.Album, error) {
	sql, args, err := r.Builder.
		Select("id", "name", "has_cover", "COALESCE(cover_image_id, 0)").
		From("albums").
		OrderBy("RANDOM()").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandom - r.Builder: %w", err)
	}

	var a entity.Album
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&a.ID, &a.Name, &a.HasCover, &a.CoverImageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandom - no albums found")
		}
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandom - QueryRow: %w", err)
	}
	return a, nil
}

// GetRandomExcludeRecent returns a random album not found among the excludeN
// most-recently-sent albums (ordered by last_sent_at DESC).
// When all albums have been sent within the history window (no eligible row),
// it falls back to GetRandom so the scheduler never stalls.
func (r *AlbumsRepo) GetRandomExcludeRecent(ctx context.Context, excludeN int) (entity.Album, error) {
	sql, args, err := r.Builder.
		Select("id", "name", "has_cover", "COALESCE(cover_image_id, 0)").
		From("albums").
		Where("id NOT IN (SELECT id FROM albums WHERE last_sent_at IS NOT NULL ORDER BY last_sent_at DESC LIMIT ?)", excludeN).
		OrderBy("RANDOM()").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandomExcludeRecent - r.Builder: %w", err)
	}

	var a entity.Album
	err = r.Pool.QueryRow(ctx, sql, args...).Scan(&a.ID, &a.Name, &a.HasCover, &a.CoverImageID)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandomExcludeRecent - QueryRow: %w", err)
	}
	// All albums are within the history window — reset by falling back to fully random.
	return r.GetRandom(ctx)
}

// MarkSent stamps last_sent_at = NOW() for the given album.
func (r *AlbumsRepo) MarkSent(ctx context.Context, albumID int) error {
	sql, args, err := r.Builder.
		Update("albums").
		Set("last_sent_at", sq.Expr("NOW()")).
		Where("id = ?", albumID).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - MarkSent - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - MarkSent - Exec: %w", err)
	}
	return nil
}

// IncrRating increments positive_rating by 1 for albumID.
func (r *AlbumsRepo) IncrRating(ctx context.Context, albumID int) error {
	sql, args, err := r.Builder.
		Update("albums").
		Set("positive_rating", sq.Expr("positive_rating + 1")).
		Where("id = ?", albumID).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - IncrRating - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - IncrRating - Exec: %w", err)
	}
	return nil
}

// SetCover marks an album as having a cover and records which image is the cover.
func (r *AlbumsRepo) SetCover(ctx context.Context, albumID, coverImageID int) error {
	sql, args, err := r.Builder.
		Update("albums").
		Set("has_cover", true).
		Set("cover_image_id", coverImageID).
		Where("id = ?", albumID).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - SetCover - r.Builder: %w", err)
	}

	_, err = r.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("AlbumsRepo - SetCover - Exec: %w", err)
	}
	return nil
}

// ClearCover removes the cover designation from an album.
func (r *AlbumsRepo) ClearCover(ctx context.Context, albumID int) error {
	sql, args, err := r.Builder.
		Update("albums").
		Set("has_cover", false).
		Set("cover_image_id", nil).
		Where("id = ?", albumID).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - ClearCover - r.Builder: %w", err)
	}

	_, err = r.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("AlbumsRepo - ClearCover - Exec: %w", err)
	}
	return nil
}
