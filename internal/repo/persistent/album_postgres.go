package persistent

import (
	"context"
	"errors"
	"fmt"

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
