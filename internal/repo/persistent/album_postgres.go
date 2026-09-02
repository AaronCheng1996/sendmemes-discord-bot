package persistent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

// AlbumsRepo -.
type AlbumsRepo struct {
	*postgres.Postgres
}

// NewAlbumsRepo -.
func NewAlbumsRepo(pg *postgres.Postgres) *AlbumsRepo {
	return &AlbumsRepo{Postgres: pg}
}

// albumColumns is the album projection shared by every read and every RETURNING
// clause, in the order scanAlbumRow expects. Kept in one place so a new column
// cannot reach one query and miss another.
func albumColumns() []string {
	return []string{
		"id",
		"name",
		"COALESCE(folder_id, 0)",
		"COALESCE(source_path, '')",
		"has_cover",
		"COALESCE(cover_image_id, 0)",
		"send_mode",
		"COALESCE(send_config_json::text, '')",
		"last_sent_at",
		"COALESCE(positive_rating, 0)",
		"missing_since",
	}
}

// albumReturning renders albumColumns as a RETURNING suffix.
func albumReturning() string {
	return "RETURNING " + strings.Join(albumColumns(), ", ")
}

func scanAlbumRow(row pgx.Row) (entity.Album, error) {
	var a entity.Album
	var lastSentAt, missingSince *time.Time
	if err := row.Scan(
		&a.ID,
		&a.Name,
		&a.FolderID,
		&a.SourcePath,
		&a.HasCover,
		&a.CoverImageID,
		&a.SendMode,
		&a.SendConfigJSON,
		&lastSentAt,
		&a.PositiveRating,
		&missingSince,
	); err != nil {
		return entity.Album{}, err
	}
	a.LastSentAt = lastSentAt
	a.MissingSince = missingSince
	return a, nil
}

func albumSelectBuilder(r *AlbumsRepo) sq.SelectBuilder {
	return r.Builder.Select(albumColumns()...).From("albums")
}

func (r *AlbumsRepo) albumAdminOrderBy(q repo.AlbumAdminListQuery) string {
	dir := "DESC"
	if q.SortAsc {
		dir = "ASC"
	}
	switch strings.ToLower(strings.TrimSpace(q.SortBy)) {
	case "name":
		return "name " + dir + ", id ASC"
	case "positive_rating":
		return "positive_rating " + dir + ", id ASC"
	case "cover":
		return "has_cover " + dir + ", id ASC"
	default:
		return "id " + dir
	}
}

func (r *AlbumsRepo) applyAlbumAdminFilters(b sq.SelectBuilder, q repo.AlbumAdminListQuery) sq.SelectBuilder {
	// Albums whose folder vanished are soft-deleted, not dropped: they stay out
	// of the dashboard unless it asks for them.
	if !q.IncludeMissing {
		b = b.Where("missing_since IS NULL")
	}
	raw := strings.TrimSpace(q.FilterQ)
	col := strings.ToLower(strings.TrimSpace(q.FilterCol))
	if raw == "" || col == "" {
		return b
	}
	pat := escapeILikePattern(raw)
	lraw := strings.ToLower(raw)

	switch col {
	case "id":
		return b.Where("CAST(id AS TEXT) ILIKE ?", pat)
	case "name":
		return b.Where("name ILIKE ?", pat)
	case "positive_rating":
		return b.Where("CAST(positive_rating AS TEXT) ILIKE ?", pat)
	case "cover":
		switch lraw {
		case "yes", "true", "1":
			return b.Where(sq.Eq{"has_cover": true})
		case "no", "false", "0":
			return b.Where(sq.Eq{"has_cover": false})
		default:
			return b.Where("(CASE WHEN has_cover THEN 'yes' ELSE 'no' END) ILIKE ?", pat)
		}
	case "all":
		return b.Where(albumOrFilterParts(pat, lraw))
	default:
		// Treat unknown filter_field like "all" for forward compatibility.
		return b.Where(albumOrFilterParts(pat, lraw))
	}
}

// albumPathPredicate turns a rule's path filter into SQL. Matching is
// case-insensitive and segment-wise — "Crawler" covers "Crawler/Artist" but
// never "CrawlerOld" — and mirrors entity.AlbumPathFilter.Matches, which the
// notifier applies in memory to the same paths.
//
// COALESCE keeps an album that has not been synced since source_path was added
// out of an include filter and inside an exclude one, the same way Matches does.
func albumPathPredicate(filter entity.AlbumPathFilter) sq.Sqlizer {
	f := filter.Normalized()
	if f.Mode == entity.AlbumFilterAll || len(f.Paths) == 0 {
		return nil
	}

	under := make([]sq.Sqlizer, 0, len(f.Paths))
	for _, p := range f.Paths {
		lowered := strings.ToLower(strings.Trim(p, "/"))
		under = append(under, sq.Or{
			sq.Expr("LOWER(COALESCE(source_path, '')) = ?", lowered),
			sq.Expr("LOWER(COALESCE(source_path, '')) LIKE ?", escapeLikePrefix(lowered)+"/%"),
		})
	}

	if f.Mode == entity.AlbumFilterInclude {
		return sq.Or(under)
	}

	return sq.And{sq.Expr("NOT (?)", sq.Or(under))}
}

// applyAlbumPathFilter adds the path predicate to b when the filter narrows
// anything, so an unfiltered query keeps the SQL it always had.
func applyAlbumPathFilter(b sq.SelectBuilder, filter entity.AlbumPathFilter) sq.SelectBuilder {
	if pred := albumPathPredicate(filter); pred != nil {
		return b.Where(pred)
	}
	return b
}

func albumOrFilterParts(pat, lraw string) sq.Sqlizer {
	parts := []sq.Sqlizer{
		sq.Expr("CAST(id AS TEXT) ILIKE ?", pat),
		sq.Expr("name ILIKE ?", pat),
		sq.Expr("CAST(positive_rating AS TEXT) ILIKE ?", pat),
		sq.Expr("(CASE WHEN has_cover THEN 'yes' ELSE 'no' END) ILIKE ?", pat),
	}
	switch lraw {
	case "yes", "true", "1":
		parts = append(parts, sq.Eq{"has_cover": true})
	case "no", "false", "0":
		parts = append(parts, sq.Eq{"has_cover": false})
	}
	return sq.Or(parts)
}

// List returns albums with optional admin filters/sort and pagination.
func (r *AlbumsRepo) List(ctx context.Context, q repo.AlbumAdminListQuery, offset, limit int) ([]entity.Album, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	b := albumSelectBuilder(r)
	b = r.applyAlbumAdminFilters(b, q)
	sql, args, err := b.
		OrderBy(r.albumAdminOrderBy(q)).
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

// Count returns the number of albums matching the admin list filters.
func (r *AlbumsRepo) Count(ctx context.Context, q repo.AlbumAdminListQuery) (int, error) {
	b := r.Builder.Select("COUNT(*)").From("albums")
	b = r.applyAlbumAdminFilters(b, q)
	sql, args, err := b.ToSql()
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
func (r *AlbumsRepo) Create(ctx context.Context, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Insert("albums").
		Columns("name", "send_mode", "send_config_json").
		Values(name, sendMode, sq.Expr("?::jsonb", sendConfigJSON)).
		Suffix(albumReturning()).
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

// ResolveByFolder maps a folder a sync run just walked to its album row.
//
// The folder name is the album's identity, so it is matched first: a row already
// called name is the album even when a different row currently holds folderID,
// and the id is simply rebound to it. Only when no row carries the name does
// folderID decide, and then the row holding it is the same folder under a new
// name — renaming it in place is what carries the rating, send mode and config
// across a rename instead of stranding them on an album that the missing pass
// would flag minutes later.
//
// A folder.ID of 0 means the source has no folder ids; resolution is then
// name-only, exactly as it was before folder ids existed. The walked path is
// recorded on whichever row wins, so a moved folder's rules follow it.
func (r *AlbumsRepo) ResolveByFolder(ctx context.Context, folder repo.DiscoveredFolder, defaultMode entity.AlbumSendMode) (entity.Album, repo.AlbumResolution, error) {
	album, res, err := r.resolveAlbumRow(ctx, folder, defaultMode)
	if err != nil {
		return entity.Album{}, repo.AlbumResolution{}, err
	}
	// Recorded once, after every branch: a freshly inserted row already carries
	// the path and short-circuits, and the two that matched an existing row are
	// exactly the ones whose folder may have moved since the last sync.
	if perr := r.syncSourcePath(ctx, &album, folder.Path); perr != nil {
		return entity.Album{}, repo.AlbumResolution{}, perr
	}

	return album, res, nil
}

// resolveAlbumRow is ResolveByFolder's three-way match, without the path
// bookkeeping: name, then folder id, then insert.
func (r *AlbumsRepo) resolveAlbumRow(ctx context.Context, folder repo.DiscoveredFolder, defaultMode entity.AlbumSendMode) (entity.Album, repo.AlbumResolution, error) {
	a, found, err := r.findAlbum(ctx, sq.Eq{"name": folder.Name})
	if err != nil {
		return entity.Album{}, repo.AlbumResolution{}, fmt.Errorf("AlbumsRepo - ResolveByFolder - by name %q: %w", folder.Name, err)
	}
	if found {
		if folder.ID != 0 && a.FolderID != folder.ID {
			if bindErr := r.bindFolderID(ctx, a.ID, folder.ID); bindErr != nil {
				return entity.Album{}, repo.AlbumResolution{}, bindErr
			}
			a.FolderID = folder.ID
		}

		return a, repo.AlbumResolution{}, nil
	}

	renamed, previous, found, err := r.renameFolderMatch(ctx, folder.ID, folder.Name)
	if err != nil {
		return entity.Album{}, repo.AlbumResolution{}, err
	}
	if found {
		return renamed, repo.AlbumResolution{RenamedFrom: previous}, nil
	}

	sql, args, err := r.Builder.
		Insert("albums").
		Columns("name", "folder_id", "source_path", "send_mode", "send_config_json").
		Values(folder.Name, nullableInt64(folder.ID), nullableString(folder.Path), defaultMode, sq.Expr("'{}'::jsonb")).
		Suffix(albumReturning()).
		ToSql()
	if err != nil {
		return entity.Album{}, repo.AlbumResolution{}, fmt.Errorf("AlbumsRepo - ResolveByFolder - r.Builder: %w", err)
	}
	fresh, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		return entity.Album{}, repo.AlbumResolution{}, fmt.Errorf("AlbumsRepo - ResolveByFolder - insert %q: %w", folder.Name, err)
	}

	return fresh, repo.AlbumResolution{Created: true}, nil
}

// syncSourcePath writes the walked path onto an existing album when it changed,
// keeping the album in step with a folder that moved. An empty walked path (a
// source that reports none) leaves the stored one alone rather than erasing it.
func (r *AlbumsRepo) syncSourcePath(ctx context.Context, a *entity.Album, path string) error {
	if path == "" || a.SourcePath == path {
		return nil
	}

	sql, args, err := r.Builder.
		Update("albums").
		Set("source_path", path).
		Where(sq.Eq{"id": a.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - syncSourcePath - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - syncSourcePath - Exec: %w", err)
	}
	a.SourcePath = path

	return nil
}

// renameFolderMatch renames the album holding folderID to name and returns it
// along with the name it used to have. It reports (zero, "", false, nil) when
// folderID is 0 (a source without folder ids) or no row holds it, which is the
// signal to create a fresh album instead.
func (r *AlbumsRepo) renameFolderMatch(ctx context.Context, folderID int64, name string) (album entity.Album, previousName string, found bool, err error) {
	if folderID == 0 {
		return entity.Album{}, "", false, nil
	}

	album, found, err = r.findAlbum(ctx, sq.Eq{"folder_id": folderID})
	if err != nil {
		return entity.Album{}, "", false, fmt.Errorf("AlbumsRepo - ResolveByFolder - by folder %d: %w", folderID, err)
	}
	if !found {
		return entity.Album{}, "", false, nil
	}

	previousName = album.Name
	if renameErr := r.renameAlbum(ctx, album.ID, name); renameErr != nil {
		return entity.Album{}, "", false, renameErr
	}
	album.Name = name

	return album, previousName, true, nil
}

// findAlbum returns the single album matching pred, reporting absence as
// (zero, false, nil) rather than as an error — ResolveByFolder branches on it.
func (r *AlbumsRepo) findAlbum(ctx context.Context, pred sq.Sqlizer) (entity.Album, bool, error) {
	sql, args, err := albumSelectBuilder(r).Where(pred).Limit(1).ToSql()
	if err != nil {
		return entity.Album{}, false, fmt.Errorf("r.Builder: %w", err)
	}
	a, err := scanAlbumRow(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Album{}, false, nil
		}
		return entity.Album{}, false, fmt.Errorf("QueryRow: %w", err)
	}
	return a, true, nil
}

// bindFolderID moves folderID onto albumID. Folder ids are unique, so it is
// first detached from whichever row held it: that row is a folder this run no
// longer sees under that name, and the missing pass deals with it.
func (r *AlbumsRepo) bindFolderID(ctx context.Context, albumID int, folderID int64) error {
	detach, args, err := r.Builder.
		Update("albums").
		Set("folder_id", nil).
		Where(sq.Eq{"folder_id": folderID}).
		Where(sq.NotEq{"id": albumID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - bindFolderID - detach r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, detach, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - bindFolderID - detach Exec: %w", err)
	}

	attach, args, err := r.Builder.
		Update("albums").
		Set("folder_id", folderID).
		Where(sq.Eq{"id": albumID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - bindFolderID - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, attach, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - bindFolderID - Exec: %w", err)
	}
	return nil
}

// renameAlbum changes only the name, leaving rating, send mode and config alone.
func (r *AlbumsRepo) renameAlbum(ctx context.Context, albumID int, name string) error {
	sql, args, err := r.Builder.
		Update("albums").
		Set("name", name).
		Where(sq.Eq{"id": albumID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - renameAlbum - r.Builder: %w", err)
	}
	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - renameAlbum - Exec: %w", err)
	}
	return nil
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

// GetRandom returns a random album within filter's scope.
func (r *AlbumsRepo) GetRandom(ctx context.Context, filter entity.AlbumPathFilter) (entity.Album, error) {
	sql, args, err := applyAlbumPathFilter(albumSelectBuilder(r), filter).
		Where("missing_since IS NULL").
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
func (r *AlbumsRepo) GetRandomExcludeRecent(ctx context.Context, excludeN int, filter entity.AlbumPathFilter) (entity.Album, error) {
	sql, args, err := applyAlbumPathFilter(albumSelectBuilder(r), filter).
		Where("missing_since IS NULL").
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
	// Every album in scope is within the history window — reset by falling back
	// to a fully random pick, still inside the rule's scope.
	return r.GetRandom(ctx, filter)
}

// TopRated returns up to limit albums ordered by positive_rating DESC (ties
// broken by id ASC for stable output).
func (r *AlbumsRepo) TopRated(ctx context.Context, limit int) ([]entity.Album, error) {
	if limit <= 0 {
		limit = 10
	}

	sql, args, err := albumSelectBuilder(r).
		OrderBy("positive_rating DESC, id ASC").
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("AlbumsRepo - TopRated - r.Builder: %w", err)
	}

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("AlbumsRepo - TopRated - Query: %w", err)
	}
	defer rows.Close()

	albums := make([]entity.Album, 0, limit)
	for rows.Next() {
		a, scanErr := scanAlbumRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("AlbumsRepo - TopRated - Scan: %w", scanErr)
		}
		albums = append(albums, a)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("AlbumsRepo - TopRated - rows.Err: %w", rows.Err())
	}

	return albums, nil
}

// Update changes album name by id and returns updated row.
func (r *AlbumsRepo) Update(ctx context.Context, id int, name string, sendMode entity.AlbumSendMode, sendConfigJSON string) (entity.Album, error) {
	sql, args, err := r.Builder.
		Update("albums").
		Set("name", name).
		Set("send_mode", sendMode).
		Set("send_config_json", sq.Expr("?::jsonb", sendConfigJSON)).
		Where("id = ?", id).
		Suffix(albumReturning()).
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

// MarkMissingExcept stamps missing_since = NOW() on every album whose name is
// not in seenNames, skipping albums already marked so the original timestamp is
// preserved. It returns the albums it newly marked (id and name only), which is
// also what a caller needs to record one activity event per vanished folder.
// Callers must not pass an empty slice: "nothing was seen" means the source
// failed, not that every album vanished (see the sync use case's guard).
func (r *AlbumsRepo) MarkMissingExcept(ctx context.Context, seenNames []string) ([]entity.Album, error) {
	if len(seenNames) == 0 {
		return nil, fmt.Errorf("AlbumsRepo - MarkMissingExcept: refusing to mark every album missing on an empty scan")
	}

	sql, args, err := r.Builder.
		Update("albums").
		Set("missing_since", sq.Expr("NOW()")).
		Where("missing_since IS NULL").
		Where(sq.NotEq{"name": seenNames}).
		Suffix("RETURNING id, name").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("AlbumsRepo - MarkMissingExcept - r.Builder: %w", err)
	}

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("AlbumsRepo - MarkMissingExcept - Query: %w", err)
	}
	defer rows.Close()

	var marked []entity.Album
	for rows.Next() {
		var a entity.Album
		if scanErr := rows.Scan(&a.ID, &a.Name); scanErr != nil {
			return nil, fmt.Errorf("AlbumsRepo - MarkMissingExcept - Scan: %w", scanErr)
		}
		marked = append(marked, a)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("AlbumsRepo - MarkMissingExcept - rows.Err: %w", err)
	}
	return marked, nil
}

// ClearMissing removes the missing flag from albums that were seen again, so a
// folder that comes back recovers on its own.
func (r *AlbumsRepo) ClearMissing(ctx context.Context, seenNames []string) error {
	if len(seenNames) == 0 {
		return nil
	}

	sql, args, err := r.Builder.
		Update("albums").
		Set("missing_since", nil).
		Where("missing_since IS NOT NULL").
		Where(sq.Eq{"name": seenNames}).
		ToSql()
	if err != nil {
		return fmt.Errorf("AlbumsRepo - ClearMissing - r.Builder: %w", err)
	}

	if _, err = r.Pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("AlbumsRepo - ClearMissing - Exec: %w", err)
	}
	return nil
}
