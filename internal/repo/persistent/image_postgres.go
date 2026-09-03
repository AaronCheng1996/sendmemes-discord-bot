package persistent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

// ImagesRepo -.
type ImagesRepo struct {
	*postgres.Postgres
}

// NewImagesRepo -.
func NewImagesRepo(pg *postgres.Postgres) *ImagesRepo {
	return &ImagesRepo{Postgres: pg}
}

// imageSelectBuilder is the default projection: live rows only. Every send and
// preview path goes through it, so a soft-deleted file cannot leak back into a
// message just because a caller forgot a WHERE clause. Only the admin list
// opts out, via imageSelectBuilderScoped.
func imageSelectBuilder(r *ImagesRepo) sq.SelectBuilder {
	return imageSelectBuilderScoped(r, false)
}

func imageSelectBuilderScoped(r *ImagesRepo, includeDeleted bool) sq.SelectBuilder {
	b := r.Builder.
		Select("i.id", "i.url", "i.source", "i.file_id", "i.album_id", "COALESCE(i.guild_id, '')", "COALESCE(a.name, '')", "i.kind", "COALESCE(i.size_bytes, 0)", "COALESCE(i.public_link, '')", "i.deleted_at").
		From("images i").
		LeftJoin("albums a ON a.id = i.album_id")
	if !includeDeleted {
		b = b.Where(liveImages)
	}
	return b
}

// liveImages is the "not soft-deleted" predicate, spelled once so the aggregate
// queries that build their own statements stay in step with the projection.
const liveImages = "i.deleted_at IS NULL"

func scanImageRow(row pgx.Row) (entity.Image, error) {
	var e entity.Image
	var source *string
	var fileID *int64
	var albumID *int
	var guildID string
	if err := row.Scan(&e.ID, &e.URL, &source, &fileID, &albumID, &guildID, &e.AlbumName, &e.Kind, &e.SizeBytes, &e.PublicLink, &e.DeletedAt); err != nil {
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

func (r *ImagesRepo) imageAdminOrderBy(q repo.ImageAdminListQuery) string {
	dir := "DESC"
	if q.SortAsc {
		dir = "ASC"
	}
	switch strings.ToLower(strings.TrimSpace(q.SortBy)) {
	case "album_id":
		return "i.album_id " + dir + ", i.id ASC"
	case "url":
		return "i.url " + dir + ", i.id ASC"
	case "source":
		return "COALESCE(i.source, '') " + dir + ", i.id ASC"
	case "guild_id":
		return "COALESCE(i.guild_id, '') " + dir + ", i.id ASC"
	case "file_id":
		return "COALESCE(i.file_id, 0) " + dir + ", i.id ASC"
	default:
		return "i.id " + dir
	}
}

func (r *ImagesRepo) applyImageAdminFilters(b sq.SelectBuilder, q repo.ImageAdminListQuery) sq.SelectBuilder {
	if q.AlbumScopeID > 0 {
		b = b.Where(sq.Eq{"i.album_id": q.AlbumScopeID})
	}
	raw := strings.TrimSpace(q.FilterQ)
	col := strings.ToLower(strings.TrimSpace(q.FilterCol))
	if raw == "" || col == "" {
		return b
	}
	pat := escapeILikePattern(raw)

	switch col {
	case "id":
		return b.Where("CAST(i.id AS TEXT) ILIKE ?", pat)
	case "album_id":
		return b.Where("CAST(COALESCE(i.album_id, 0) AS TEXT) ILIKE ?", pat)
	case "url":
		return b.Where("i.url ILIKE ?", pat)
	case "source":
		return b.Where("COALESCE(i.source, '') ILIKE ?", pat)
	case "guild_id":
		return b.Where("COALESCE(i.guild_id, '') ILIKE ?", pat)
	case "file_id":
		return b.Where("CAST(COALESCE(i.file_id, 0) AS TEXT) ILIKE ?", pat)
	case "all":
		return b.Where(imageOrFilterParts(pat))
	default:
		return b.Where(imageOrFilterParts(pat))
	}
}

func imageOrFilterParts(pat string) sq.Sqlizer {
	return sq.Or{
		sq.Expr("CAST(i.id AS TEXT) ILIKE ?", pat),
		sq.Expr("CAST(COALESCE(i.album_id, 0) AS TEXT) ILIKE ?", pat),
		sq.Expr("i.url ILIKE ?", pat),
		sq.Expr("COALESCE(i.source, '') ILIKE ?", pat),
		sq.Expr("COALESCE(i.guild_id, '') ILIKE ?", pat),
		sq.Expr("CAST(COALESCE(i.file_id, 0) AS TEXT) ILIKE ?", pat),
	}
}

// List returns images with optional album scope, filters, sort, and pagination.
func (r *ImagesRepo) List(ctx context.Context, q repo.ImageAdminListQuery, offset, limit int) ([]entity.Image, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	b := imageSelectBuilderScoped(r, q.IncludeDeleted)
	b = r.applyImageAdminFilters(b, q)
	sql, args, err := b.
		OrderBy(r.imageAdminOrderBy(q)).
		Offset(uint64(offset)).
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - List - r.Builder: %w", err)
	}
	return r.queryImages(ctx, "ImagesRepo - List", sql, args)
}

// Count returns the number of images matching the admin list query.
func (r *ImagesRepo) Count(ctx context.Context, q repo.ImageAdminListQuery) (int, error) {
	b := r.Builder.Select("COUNT(*)").From("images i")
	if !q.IncludeDeleted {
		b = b.Where(liveImages)
	}
	b = r.applyImageAdminFilters(b, q)
	sql, args, err := b.ToSql()
	if err != nil {
		return 0, fmt.Errorf("ImagesRepo - Count - r.Builder: %w", err)
	}
	var n int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("ImagesRepo - Count - QueryRow: %w", err)
	}
	return n, nil
}

// CountByKind returns the number of images with the given kind
// (entity.MediaKindImage or entity.MediaKindVideo).
func (r *ImagesRepo) CountByKind(ctx context.Context, kind string) (int, error) {
	sql, args, err := r.Builder.
		Select("COUNT(*)").
		From("images").
		Where(sq.Eq{"kind": kind}).
		Where("deleted_at IS NULL").
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("ImagesRepo - CountByKind - r.Builder: %w", err)
	}
	var n int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("ImagesRepo - CountByKind - QueryRow: %w", err)
	}
	return n, nil
}

// CountAlbumMedia returns albumID's image and video counts. Both come back from
// one round trip because the captions that mention one usually mention the other.
func (r *ImagesRepo) CountAlbumMedia(ctx context.Context, albumID int) (int, int, error) {
	sql, args, err := r.Builder.
		Select().
		Column("COUNT(*) FILTER (WHERE kind = ?)", entity.MediaKindImage).
		Column("COUNT(*) FILTER (WHERE kind = ?)", entity.MediaKindVideo).
		From("images").
		Where(sq.Eq{"album_id": albumID}).
		Where("deleted_at IS NULL").
		ToSql()
	if err != nil {
		return 0, 0, fmt.Errorf("ImagesRepo - CountAlbumMedia - r.Builder: %w", err)
	}
	var images, videos int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&images, &videos); err != nil {
		return 0, 0, fmt.Errorf("ImagesRepo - CountAlbumMedia - QueryRow: %w", err)
	}
	return images, videos, nil
}

// GetFirstByAlbum returns the lowest-id image in albumID, used as a preview when
// the album has no explicit cover. Returns (zero, false, nil) when the album has no images.
func (r *ImagesRepo) GetFirstByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error) {
	sql, args, err := imageSelectBuilder(r).
		Where(sq.Eq{"i.album_id": albumID}).
		Where("i.kind = 'image'").
		OrderBy("i.id ASC").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - GetFirstByAlbum - r.Builder: %w", err)
	}
	e, err := scanImageRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Image{}, false, nil
		}
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - GetFirstByAlbum - QueryRow: %w", err)
	}
	return e, true, nil
}

// GetByID returns image by primary key, soft-deleted rows included: it backs the
// admin's explicit by-id lookups, which must still be able to open a row the
// dashboard is showing under "include deleted".
func (r *ImagesRepo) GetByID(ctx context.Context, id int) (entity.Image, error) {
	sql, args, err := imageSelectBuilderScoped(r, true).Where("i.id = ?", id).Limit(1).ToSql()
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
		Where("kind = 'image'").
		Where("deleted_at IS NULL").
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
	sql, args, err := imageSelectBuilder(r).Where("i.kind = 'image'").OrderBy("RANDOM()").Limit(1).ToSql()
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetRandom - r.Builder: %w", err)
	}

	e, err := scanImageRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesRepo - GetRandom - QueryRow: %w", err)
	}
	return e, nil
}

// GetRandomByAlbum returns up to limit random media rows from the given album.
// Pass excludeID > 0 to exclude a specific row (e.g. the cover) from the result.
//
// Videos are included: Discord renders an uploaded video as an inline player,
// so outside Video mode a short clip is just another attachment. Video mode has
// its own query (GetRandomVideoByAlbum) because it deliberately wants only those.
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

// GetAllByAlbum returns all media in the given album ordered by id, videos
// included (see GetRandomByAlbum).
// Pass excludeID > 0 to exclude a specific row (e.g. the cover) from the result.
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

// GetRandomVideoByAlbum returns one random video (kind='video') from albumID.
// Returns (zero, false, nil) when the album has no videos.
func (r *ImagesRepo) GetRandomVideoByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error) {
	sql, args, err := imageSelectBuilder(r).
		Where(sq.Eq{"i.album_id": albumID}).
		Where("i.kind = 'video'").
		OrderBy("RANDOM()").
		Limit(1).
		ToSql()
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - GetRandomVideoByAlbum - r.Builder: %w", err)
	}

	e, err := scanImageRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Image{}, false, nil
		}
		return entity.Image{}, false, fmt.Errorf("ImagesRepo - GetRandomVideoByAlbum - QueryRow: %w", err)
	}
	return e, true, nil
}

// ListByAlbumAndNames returns the images in albumID whose filename is in names.
// Soft-deleted rows are included on purpose: a removal event names files that
// are no longer live, and its thumbnails are exactly what someone reviewing the
// activity log wants to see.
func (r *ImagesRepo) ListByAlbumAndNames(ctx context.Context, albumID int, names []string) ([]entity.Image, error) {
	if len(names) == 0 {
		return nil, nil
	}

	sql, args, err := imageSelectBuilderScoped(r, true).
		Where(sq.Eq{"i.album_id": albumID}).
		Where(sq.Eq{"i.url": names}).
		OrderBy("i.id ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - ListByAlbumAndNames - r.Builder: %w", err)
	}

	return r.queryImages(ctx, "ImagesRepo - ListByAlbumAndNames", sql, args)
}

// FindCoverByAlbum returns the image in albumID whose filename matches the cover
// convention: cover.* or _cover.* (case-insensitive).
func (r *ImagesRepo) FindCoverByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error) {
	sql, args, err := imageSelectBuilder(r).
		Where("i.album_id = ? AND i.kind = 'image' AND i.url ~* ?", albumID, `^_?cover\.`).
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
// The returned bool reports whether a new row was inserted (vs. updated);
// (xmax = 0) is true only for rows created by this statement.
//
// The conflict branch clears deleted_at, so a file that was soft-deleted and
// then reappears (a folder moved away and back keeps its file ids) is revived
// in place. That also means it comes back reported as *not* inserted, and so is
// not announced to Discord a second time — the same quiet recovery an album gets
// from ClearMissing.
func (r *ImagesRepo) UpsertByFileID(ctx context.Context, img entity.Image) (bool, error) {
	kind := img.Kind
	if kind == "" {
		kind = entity.MediaKindImage // satisfy the images.kind CHECK constraint
	}
	sql, args, err := r.Builder.
		Insert("images").
		Columns("file_id", "url", "source", "album_id", "kind", "size_bytes").
		Values(img.FileID, img.URL, img.Source, img.AlbumID, kind, nullableInt64(img.SizeBytes)).
		Suffix("ON CONFLICT (file_id) WHERE file_id IS NOT NULL DO UPDATE SET url = EXCLUDED.url, album_id = EXCLUDED.album_id, kind = EXCLUDED.kind, size_bytes = EXCLUDED.size_bytes, deleted_at = NULL RETURNING (xmax = 0)").
		ToSql()
	if err != nil {
		return false, fmt.Errorf("ImagesRepo - UpsertByFileID - r.Builder: %w", err)
	}

	var inserted bool
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&inserted); err != nil {
		return false, fmt.Errorf("ImagesRepo - UpsertByFileID - QueryRow: %w", err)
	}
	return inserted, nil
}

// SetPublicLink persists the permanent pCloud public share link for image id.
func (r *ImagesRepo) SetPublicLink(ctx context.Context, id int, link string) error {
	sql, args, err := r.Builder.
		Update("images").
		Set("public_link", link).
		Where("id = ?", id).
		ToSql()
	if err != nil {
		return fmt.Errorf("ImagesRepo - SetPublicLink - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("ImagesRepo - SetPublicLink - Exec: %w", err)
	}
	return nil
}

// SoftDeleteByAlbumNotInFileIDs flags the live rows in albumID owned by source
// whose file_id the latest walk did not report, and returns the ones it flagged.
func (r *ImagesRepo) SoftDeleteByAlbumNotInFileIDs(ctx context.Context, albumID int, source string, fileIDs []int64) ([]entity.Image, error) {
	return r.softDelete(ctx, "SoftDeleteByAlbumNotInFileIDs", sq.And{
		sq.Eq{"album_id": albumID},
		sq.Eq{"source": source},
		sq.NotEq{"file_id": fileIDs},
	})
}

// SoftDeleteByAlbum flags every live row in albumID owned by source. Used when
// the album's whole folder disappeared: its files are just as gone as the ones
// pruned out of a folder that survived, and leaving them listed would offer the
// dashboard media nothing can fetch.
func (r *ImagesRepo) SoftDeleteByAlbum(ctx context.Context, albumID int, source string) ([]entity.Image, error) {
	return r.softDelete(ctx, "SoftDeleteByAlbum", sq.And{
		sq.Eq{"album_id": albumID},
		sq.Eq{"source": source},
	})
}

// softDelete stamps deleted_at on the live rows matching pred and returns them.
// The "live rows only" clause both keeps an earlier removal's timestamp intact
// and keeps the result to what actually changed, so a caller can record one
// activity event per removal instead of one per sync run.
func (r *ImagesRepo) softDelete(ctx context.Context, caller string, pred sq.Sqlizer) ([]entity.Image, error) {
	sqlStr, args, err := r.Builder.
		Update("images").
		Set("deleted_at", sq.Expr("NOW()")).
		Where(pred).
		Where("deleted_at IS NULL").
		Suffix("RETURNING id, url, kind").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - %s - r.Builder: %w", caller, err)
	}

	rows, err := r.Pool.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("ImagesRepo - %s - Query: %w", caller, err)
	}
	defer rows.Close()

	var removed []entity.Image
	for rows.Next() {
		var e entity.Image
		if scanErr := rows.Scan(&e.ID, &e.URL, &e.Kind); scanErr != nil {
			return nil, fmt.Errorf("ImagesRepo - %s - Scan: %w", caller, scanErr)
		}
		removed = append(removed, e)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("ImagesRepo - %s - rows.Err: %w", caller, err)
	}
	return removed, nil
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
