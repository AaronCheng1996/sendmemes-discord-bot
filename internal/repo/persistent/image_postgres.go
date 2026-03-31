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

func imageSelectBuilder(r *ImagesRepo) sq.SelectBuilder {
	return r.Builder.
		Select("i.id", "i.url", "i.source", "i.file_id", "i.album_id", "COALESCE(i.guild_id, '')", "COALESCE(a.name, '')").
		From("images i").
		LeftJoin("albums a ON a.id = i.album_id")
}

func scanImageRow(row pgx.Row) (entity.Image, error) {
	var e entity.Image
	var source *string
	var fileID *int64
	var albumID *int
	var guildID string
	if err := row.Scan(&e.ID, &e.URL, &source, &fileID, &albumID, &guildID, &e.AlbumName); err != nil {
		return entity.Image{}, err
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
	e.GuildID = guildID
	return e, nil
}

// List returns images with optional album filter and pagination.
func (r *ImagesRepo) List(ctx context.Context, albumID, offset, limit int) ([]entity.Image, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := imageSelectBuilder(r).OrderBy("i.id ASC").Offset(uint64(offset)).Limit(uint64(limit))
	if albumID > 0 {
		q = q.Where(sq.Eq{"i.album_id": albumID})
	}

	sql, args, err := q.ToSql()
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - List - r.Builder: %w", err)
	}
	return r.queryImages(ctx, "ImagesRepo - List", sql, args)
}

// GetByID returns image by primary key.
func (r *ImagesRepo) GetByID(ctx context.Context, id int) (entity.Image, error) {
	sql, args, err := imageSelectBuilder(r).Where("i.id = ?", id).Limit(1).ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetByID - r.Builder: %w", err)
	}
	e, err := scanImageRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Image{}, fmt.Errorf("ImagesRepo - GetByID - image %d not found", id)
		}
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetByID - QueryRow: %w", err)
	}
	return e, nil
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
	sql, args, err := imageSelectBuilder(r).OrderBy("RANDOM()").Limit(1).ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetRandom - r.Builder: %w", err)
	}

	e, err := scanImageRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetRandom - QueryRow: %w", err)
	}
	return e, nil
}

// GetRandomByAlbum returns up to limit random images from the given album.
// Pass excludeID > 0 to exclude a specific image (e.g. the cover) from the result.
func (r *ImagesRepo) GetRandomByAlbum(ctx context.Context, albumID, limit, excludeID int) ([]entity.Image, error) {
	q := imageSelectBuilder(r).Where(sq.Eq{"i.album_id": albumID}).OrderBy("RANDOM()").Limit(uint64(limit))

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
	q := imageSelectBuilder(r).Where(sq.Eq{"i.album_id": albumID}).OrderBy("i.id ASC")

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
	sql, args, err := imageSelectBuilder(r).
		Where("i.album_id = ? AND i.url ~* ?", albumID, `^_?cover\.`).
		Limit(1).ToSql()
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - FindCoverByAlbum - r.Builder: %w", err)
	}

	e, err := scanImageRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Image{}, false, nil
		}
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - FindCoverByAlbum - QueryRow: %w", err)
	}
	return e, true, nil
}

// Insert inserts one image row.
func (r *ImagesRepo) Insert(ctx context.Context, img entity.Image) (entity.Image, error) {
	sql, args, err := r.Builder.
		Insert("images").
		Columns("url", "source", "guild_id", "album_id", "file_id").
		Values(img.URL, nullableString(img.Source), nullableString(img.GuildID), nullableInt(img.AlbumID), nullableInt64(img.FileID)).
		Suffix("RETURNING id, url, COALESCE(source, ''), COALESCE(guild_id, ''), COALESCE(album_id, 0), COALESCE(file_id, 0)").
		ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - Insert - r.Builder: %w", err)
	}

	var out entity.Image
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&out.ID, &out.URL, &out.Source, &out.GuildID, &out.AlbumID, &out.FileID); err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - Insert - QueryRow: %w", err)
	}
	return out, nil
}

// Update updates image fields and returns updated row.
func (r *ImagesRepo) Update(ctx context.Context, img entity.Image) (entity.Image, error) {
	sql, args, err := r.Builder.
		Update("images").
		Set("url", img.URL).
		Set("source", nullableString(img.Source)).
		Set("guild_id", nullableString(img.GuildID)).
		Set("album_id", nullableInt(img.AlbumID)).
		Set("file_id", nullableInt64(img.FileID)).
		Where("id = ?", img.ID).
		Suffix("RETURNING id, url, COALESCE(source, ''), COALESCE(guild_id, ''), COALESCE(album_id, 0), COALESCE(file_id, 0)").
		ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - Update - r.Builder: %w", err)
	}

	var out entity.Image
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&out.ID, &out.URL, &out.Source, &out.GuildID, &out.AlbumID, &out.FileID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Image{}, fmt.Errorf("ImagesRepo - Update - image %d not found", img.ID)
		}
		return entity.Image{}, fmt.Errorf("ImagesRepo - Update - QueryRow: %w", err)
	}
	return out, nil
}

// Delete removes image by id.
func (r *ImagesRepo) Delete(ctx context.Context, id int) error {
	sql, args, err := r.Builder.Delete("images").Where("id = ?", id).ToSql()
	if err != nil {
		return fmt.Errorf("ImagesRepo - Delete - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("ImagesRepo - Delete - Exec: %w", err)
	}
	return nil
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
		e, scanErr := scanImageRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("%s - Scan: %w", caller, scanErr)
		}
		images = append(images, e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s - rows.Err: %w", caller, err)
	}
	return images, nil
}

func nullableString(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

func nullableInt(v int) interface{} {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableInt64(v int64) interface{} {
	if v <= 0 {
		return nil
	}
	return v
}
