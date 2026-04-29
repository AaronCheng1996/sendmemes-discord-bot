package persistent

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func scanAlbumRow(row pgx.Row) (entity.Album, error) {
	var a entity.Album
	var lastSentAt *time.Time
	if err := row.Scan(
		&a.ID,
		&a.Name,
		&a.HasCover,
		&a.CoverImageID,
		&lastSentAt,
		&a.PositiveRating,
	); err != nil {
		return entity.Album{}, err
	}
	a.LastSentAt = lastSentAt
	return a, nil
}

func albumSelectBuilder(r *AlbumsRepo) sq.SelectBuilder {
	return r.Builder.
		Select(
			"id",
			"name",
			"has_cover",
			"COALESCE(cover_image_id, 0)",
			"last_sent_at",
			"COALESCE(positive_rating, 0)",
		).
		From("albums")
}

// List returns albums ordered by id with pagination.
func (r *AlbumsRepo) List(ctx context.Context, offset, limit int) ([]entity.Album, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	sql, args, err := albumSelectBuilder(r).
		OrderBy("id ASC").
		Offset(uint64(offset)).
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("AlbumsRepo - List - r.Builder: %w", err)
	}

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("AlbumsRepo - List - Query: %w", err)
	}
	defer rows.Close()

	albums := make([]entity.Album, 0, limit)
	for rows.Next() {
		a, scanErr := scanAlbumRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("AlbumsRepo - List - Scan: %w", scanErr)
		}
		albums = append(albums, a)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("AlbumsRepo - List - rows.Err: %w", rows.Err())
	}

	return albums, nil
}

// Count returns the total number of albums.
func (r *AlbumsRepo) Count(ctx context.Context) (int, error) {
	sql, args, err := r.Builder.Select("COUNT(*)").From("albums").ToSql()
	if err != nil {
		return 0, fmt.Errorf("AlbumsRepo - Count - r.Builder: %w", err)
	}
	var n int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("AlbumsRepo - Count - QueryRow: %w", err)
	}
	return n, nil
}

// GetByID returns album by primary key.
func (r *AlbumsRepo) GetByID(ctx context.Context, id int) (entity.Album, error) {
	sql, args, err := albumSelectBuilder(r).
		Where("id = ?", id).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByID - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByID - album %d not found", id)
		}
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByID - QueryRow: %w", err)
	}
	return a, nil
}

// Create inserts a new album.
func (r *AlbumsRepo) Create(ctx context.Context, name string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Insert("albums").
		Columns("name").
		Values(name).
		Suffix("RETURNING id, name, has_cover, COALESCE(cover_image_id, 0), last_sent_at, COALESCE(positive_rating, 0)").
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - Create - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - Create - QueryRow: %w", err)
	}
	return a, nil
}

// GetOrCreate returns the album with the given name, creating it if it does not exist.
func (r *AlbumsRepo) GetOrCreate(ctx context.Context, name string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Insert("albums").
		Columns("name").
		Values(name).
		Suffix("ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id, name, has_cover, COALESCE(cover_image_id, 0), last_sent_at, COALESCE(positive_rating, 0)").
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetOrCreate - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetOrCreate - QueryRow: %w", err)
	}
	return a, nil
}

// GetByName returns the album with the given name.
func (r *AlbumsRepo) GetByName(ctx context.Context, name string) (entity.Album, error) {
	sql, args, err := albumSelectBuilder(r).
		Where("name = ?", name).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByName - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByName - album %q not found", name)
		}
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetByName - QueryRow: %w", err)
	}
	return a, nil
}

// GetRandom returns a random album.
func (r *AlbumsRepo) GetRandom(ctx context.Context) (entity.Album, error) {
	sql, args, err := albumSelectBuilder(r).
		OrderBy("RANDOM()").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandom - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
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
	sql, args, err := albumSelectBuilder(r).
		Where("id NOT IN (SELECT id FROM albums WHERE last_sent_at IS NOT NULL ORDER BY last_sent_at DESC LIMIT ?)", excludeN).
		OrderBy("RANDOM()").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandomExcludeRecent - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - GetRandomExcludeRecent - QueryRow: %w", err)
	}
	// All albums are within the history window — reset by falling back to fully random.
	return r.GetRandom(ctx)
}

// Update changes album name by id and returns updated row.
func (r *AlbumsRepo) Update(ctx context.Context, id int, name string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Update("albums").
		Set("name", name).
		Where("id = ?", id).
		Suffix("RETURNING id, name, has_cover, COALESCE(cover_image_id, 0), last_sent_at, COALESCE(positive_rating, 0)").
		ToSql()
	if err != nil {
		return entity.Album{}, fmt.Errorf("AlbumsRepo - Update - r.Builder: %w", err)
	}

	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Album{}, fmt.Errorf("AlbumsRepo - Update - album %d not found", id)
		}
		return entity.Album{}, fmt.Errorf("AlbumsRepo - Update - QueryRow: %w", err)
	}
	return a, nil
}

// Delete removes album by id.
func (r *AlbumsRepo) Delete(ctx context.Context, id int) error {
	sql, args, err := r.Builder.
		Delete("albums").
		Where("id = ?", id).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - Delete - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - Delete - Exec: %w", err)
	}
	return nil
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
