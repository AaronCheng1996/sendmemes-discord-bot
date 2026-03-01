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

// ImagesRepo -.
type ImagesRepo struct {
	*postgres.Postgres
}

// NewImagesRepo -.
func NewImagesRepo(pg *postgres.Postgres) *ImagesRepo {
	return &ImagesRepo{Postgres: pg}
}

// GetDefault returns the default image (first row by id).
func (r *ImagesRepo) GetDefault(ctx context.Context) (entity.Image, error) {
	sql, args, err := r.Builder.
		Select("id", "url", "source", "guild_id").
		From("images").
		OrderBy("id ASC").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetDefault - r.Builder: %w", err)
	}

	var e entity.Image
	var source, guildID *string
	err = r.Pool.QueryRow(ctx, sql, args...).Scan(&e.ID, &e.URL, &source, &guildID)
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetDefault - QueryRow: %w", err)
	}
	if source != nil {
		e.Source = *source
	}
	if guildID != nil {
		e.GuildID = *guildID
	}
	return e, nil
}

// GetRandom returns a single random image from all images.
func (r *ImagesRepo) GetRandom(ctx context.Context) (entity.Image, error) {
	sql, args, err := r.Builder.
		Select("i.id", "i.url", "i.source", "i.file_id", "i.album_id", "COALESCE(a.name, '')").
		From("images i").
		LeftJoin("albums a ON a.id = i.album_id").
		OrderBy("RANDOM()").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetRandom - r.Builder: %w", err)
	}

	var e entity.Image
	var source *string
	var albumID *int
	var fileID *int64
	err = r.Pool.QueryRow(ctx, sql, args...).Scan(&e.ID, &e.URL, &source, &fileID, &albumID, &e.AlbumName)
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetRandom - QueryRow: %w", err)
	}
	if source != nil {
		e.Source = *source
	}
	if fileID != nil {
		e.FileID = *fileID
	}
	if albumID != nil {
		e.AlbumID = *albumID
	}
	return e, nil
}

// GetRandomByAlbum returns up to limit random images from the given album.
// Pass excludeID > 0 to exclude a specific image (e.g. the cover) from the result.
func (r *ImagesRepo) GetRandomByAlbum(ctx context.Context, albumID, limit, excludeID int) ([]entity.Image, error) {
	q := r.Builder.
		Select("i.id", "i.url", "i.source", "i.file_id", "i.album_id", "COALESCE(a.name, '')").
		From("images i").
		LeftJoin("albums a ON a.id = i.album_id").
		Where(sq.Eq{"i.album_id": albumID}).
		OrderBy("RANDOM()").
		Limit(uint64(limit))

	if excludeID > 0 {
		q = q.Where(sq.NotEq{"i.id": excludeID})
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - GetRandomByAlbum - r.Builder: %w", err)
	}
	return r.queryImages(ctx, "ImagesRepo - GetRandomByAlbum", sql, args)
}

// GetAllByAlbum returns all images in the given album ordered by id.
// Pass excludeID > 0 to exclude a specific image (e.g. the cover) from the result.
func (r *ImagesRepo) GetAllByAlbum(ctx context.Context, albumID, excludeID int) ([]entity.Image, error) {
	q := r.Builder.
		Select("i.id", "i.url", "i.source", "i.file_id", "i.album_id", "COALESCE(a.name, '')").
		From("images i").
		LeftJoin("albums a ON a.id = i.album_id").
		Where(sq.Eq{"i.album_id": albumID}).
		OrderBy("i.id ASC")

	if excludeID > 0 {
		q = q.Where(sq.NotEq{"i.id": excludeID})
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - GetAllByAlbum - r.Builder: %w", err)
	}
	return r.queryImages(ctx, "ImagesRepo - GetAllByAlbum", sql, args)
}

// FindCoverByAlbum returns the image in albumID whose filename matches the cover
// convention: cover.* or _cover.* (case-insensitive).
func (r *ImagesRepo) FindCoverByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error) {
	sql, args, err := r.Builder.
		Select("i.id", "i.url", "i.source", "i.file_id", "i.album_id", "COALESCE(a.name, '')").
		From("images i").
		LeftJoin("albums a ON a.id = i.album_id").
		Where("i.album_id = ? AND i.url ~* ?", albumID, `^_?cover\.`).
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - FindCoverByAlbum - r.Builder: %w", err)
	}

	var e entity.Image
	var source *string
	var fileID *int64
	var albumIDVal *int
	err = r.Pool.QueryRow(ctx, sql, args...).Scan(&e.ID, &e.URL, &source, &fileID, &albumIDVal, &e.AlbumName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Image{}, false, nil
		}
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - FindCoverByAlbum - QueryRow: %w", err)
	}
	if source != nil {
		e.Source = *source
	}
	if fileID != nil {
		e.FileID = *fileID
	}
	if albumIDVal != nil {
		e.AlbumID = *albumIDVal
	}
	return e, true, nil
}

// UpsertByFileID inserts or updates an image record keyed on file_id.
func (r *ImagesRepo) UpsertByFileID(ctx context.Context, img entity.Image) error {
	sql, args, err := r.Builder.
		Insert("images").
		Columns("file_id", "url", "source", "album_id").
		Values(img.FileID, img.URL, img.Source, img.AlbumID).
		Suffix("ON CONFLICT (file_id) WHERE file_id IS NOT NULL DO UPDATE SET url = EXCLUDED.url, album_id = EXCLUDED.album_id").
		ToSql()
	if err != nil {
		return fmt.Errorf("ImagesRepo - UpsertByFileID - r.Builder: %w", err)
	}

	_, err = r.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("ImagesRepo - UpsertByFileID - Exec: %w", err)
	}
	return nil
}

// DeleteByAlbumNotInFileIDs removes pCloud images in albumID whose file_id is not in fileIDs.
func (r *ImagesRepo) DeleteByAlbumNotInFileIDs(ctx context.Context, albumID int, fileIDs []int64) error {
	q := r.Builder.
		Delete("images").
		Where(sq.And{
			sq.Eq{"album_id": albumID},
			sq.Eq{"source": "pcloud"},
			sq.NotEq{"file_id": fileIDs},
		})

	sqlStr, args, err := q.ToSql()
	if err != nil {
		return fmt.Errorf("ImagesRepo - DeleteByAlbumNotInFileIDs - r.Builder: %w", err)
	}

	_, err = r.Pool.Exec(ctx, sqlStr, args...)
	if err != nil {
		return fmt.Errorf("ImagesRepo - DeleteByAlbumNotInFileIDs - Exec: %w", err)
	}
	return nil
}

// queryImages is a shared scanner helper for multi-row image queries.
func (r *ImagesRepo) queryImages(ctx context.Context, caller, sqlStr string, args []interface{}) ([]entity.Image, error) {
	rows, err := r.Pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("%s - Query: %w", caller, err)
	}
	defer rows.Close()

	var images []entity.Image
	for rows.Next() {
		var e entity.Image
		var source *string
		var fileID *int64
		var albumID *int
		if err = rows.Scan(&e.ID, &e.URL, &source, &fileID, &albumID, &e.AlbumName); err != nil {
			return nil, fmt.Errorf("%s - Scan: %w", caller, err)
		}
		if source != nil {
			e.Source = *source
		}
		if fileID != nil {
			e.FileID = *fileID
		}
		if albumID != nil {
			e.AlbumID = *albumID
		}
		images = append(images, e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s - rows.Err: %w", caller, err)
	}
	return images, nil
}
