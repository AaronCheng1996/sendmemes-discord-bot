// Package sync implements the pCloud image synchronization use case.
package sync

import (
	"context"
	"fmt"
	"sort"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

const (
	// maxEventFileNames caps how many discovered file names are sampled per event.
	maxEventFileNames = 10
	// maxEventMedia caps how many new media records are carried in-memory per
	// event for the Discord notifier (bounds message size / memory).
	maxEventMedia = 50
)

// UseCase synchronizes a media source's file tree with the database.
type UseCase struct {
	source      repo.MediaSource
	sourceName  string // entity.MediaSourcePCloud or entity.MediaSourceLocal; stamped on synced images
	albums      repo.AlbumsRepo
	images      repo.ImagesRepo
	events      repo.SyncEventsRepo
	defaultMode entity.AlbumSendMode // send_mode for albums created during sync
}

// New creates a new sync use case. defaultMode is the send_mode assigned to
// albums newly created during a sync (existing albums keep their stored mode).
// sourceName identifies source (entity.MediaSourcePCloud/MediaSourceLocal) and
// is stamped on every synced image's Source field, and used to scope pruning
// to rows owned by this source.
func New(source repo.MediaSource, sourceName string, albums repo.AlbumsRepo, images repo.ImagesRepo, events repo.SyncEventsRepo, defaultMode entity.AlbumSendMode) *UseCase {
	return &UseCase{
		source:      source,
		sourceName:  sourceName,
		albums:      albums,
		images:      images,
		events:      events,
		defaultMode: defaultMode,
	}
}

// albumSyncStats accumulates per-album discovery counters for one run.
type albumSyncStats struct {
	albumID   int
	created   bool
	newImages int
	newVideos int
	fileNames []string
	newMedia  []entity.Image
}

// SyncImages fetches the full pCloud folder tree and reconciles it with the database:
//  1. For each discovered media file, upsert the album and the file row.
//  2. Remove DB rows for files that no longer exist in pCloud (per album).
//  3. Detect cover images (filename matches cover.* or _cover.*) and update album.has_cover.
//  4. Record one sync event per album that gained new content and return them in
//     a SyncReport (InitialImport is set when the database had no albums before
//     this run, so callers can suppress notifications on first import).
func (uc *UseCase) SyncImages(ctx context.Context) (entity.SyncReport, error) {
	priorAlbums, err := uc.albums.Count(ctx, repo.AlbumAdminListQuery{})
	if err != nil {
		return entity.SyncReport{}, fmt.Errorf("SyncUseCase - SyncImages - albums.Count: %w", err)
	}
	report := entity.SyncReport{InitialImport: priorAlbums == 0}

	entries, err := uc.source.ListMedia(ctx)
	if err != nil {
		return report, fmt.Errorf("SyncUseCase - SyncImages - ListMedia: %w", err)
	}

	// Group file IDs per album name so we can prune stale rows and detect covers
	// after upsert, and track per-album discovery stats for the sync report.
	albumFileIDs := make(map[string][]int64)
	stats := make(map[string]*albumSyncStats)

	for _, entry := range entries {
		album, created, err := uc.albums.GetOrCreate(ctx, entry.ParentFolderName, uc.defaultMode)
		if err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - GetOrCreate album %q: %w", entry.ParentFolderName, err)
		}

		st := stats[entry.ParentFolderName]
		if st == nil {
			st = &albumSyncStats{albumID: album.ID}
			stats[entry.ParentFolderName] = st
		}
		st.created = st.created || created

		img := entity.Image{
			FileID:    entry.FileID,
			URL:       entry.Name, // store filename; full link resolved at send time via the MediaSource
			Source:    uc.sourceName,
			AlbumID:   album.ID,
			Kind:      entry.Kind,
			SizeBytes: entry.Size,
		}
		inserted, err := uc.images.UpsertByFileID(ctx, img)
		if err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - UpsertByFileID fileID=%d: %w", entry.FileID, err)
		}
		if inserted {
			if entry.Kind == entity.MediaKindVideo {
				st.newVideos++
			} else {
				st.newImages++
			}
			if len(st.fileNames) < maxEventFileNames {
				st.fileNames = append(st.fileNames, entry.Name)
			}
			if len(st.newMedia) < maxEventMedia {
				st.newMedia = append(st.newMedia, entity.Image{
					FileID:    entry.FileID,
					URL:       entry.Name,
					Source:    uc.sourceName,
					AlbumID:   st.albumID,
					AlbumName: entry.ParentFolderName, // no DB round-trip yet, so carry it from the walk
					Kind:      entry.Kind,
					SizeBytes: entry.Size,
				})
			}
		}

		albumFileIDs[entry.ParentFolderName] = append(albumFileIDs[entry.ParentFolderName], entry.FileID)
	}

	// Per-album cleanup and cover detection.
	for albumName, fileIDs := range albumFileIDs {
		album, err := uc.albums.GetByName(ctx, albumName)
		if err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - GetByName %q: %w", albumName, err)
		}

		if err = uc.images.DeleteByAlbumNotInFileIDs(ctx, album.ID, uc.sourceName, fileIDs); err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - DeleteByAlbumNotInFileIDs album %q: %w", albumName, err)
		}

		// Detect cover: look for an image whose filename matches cover.* or _cover.*
		// Cover detection is best-effort; errors do not abort the sync.
		if err = uc.updateCover(ctx, album.ID); err != nil {
			// Non-fatal: log via error return but continue processing other albums.
			_ = err // caller (scheduler) logs the returned error; we just skip this album's cover
		}
	}

	// Reconcile the missing flag: albums seen this run are cleared, albums that
	// vanished are marked (never deleted, so ratings and send config survive a
	// folder being moved away and back).
	//
	// Safety valve: a run that found nothing at all is far more likely to mean a
	// broken source, a wrong root ID or an auth problem than the user emptying
	// every folder, so skip the pass entirely rather than mark everything.
	// (A source error already aborts above; this covers "succeeded but empty".)
	if len(entries) == 0 {
		report.EmptyScan = true
	} else {
		seen := make([]string, 0, len(albumFileIDs))
		for name := range albumFileIDs {
			seen = append(seen, name)
		}

		if err = uc.albums.ClearMissing(ctx, seen); err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - ClearMissing: %w", err)
		}
		marked, merr := uc.albums.MarkMissingExcept(ctx, seen)
		if merr != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - MarkMissingExcept: %w", merr)
		}
		sort.Strings(marked)
		report.MissingAlbums = marked
	}

	// Persist one event per album with new content, ordered by album name so
	// notification output is deterministic.
	changed := make([]string, 0, len(stats))
	for name, st := range stats {
		if st.created || st.newImages > 0 || st.newVideos > 0 {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)

	for _, name := range changed {
		st := stats[name]
		eventType := entity.SyncEventFilesAdded
		if st.created {
			eventType = entity.SyncEventAlbumCreated
		}
		saved, err := uc.events.Insert(ctx, entity.SyncEvent{
			EventType: eventType,
			AlbumID:   st.albumID,
			AlbumName: name,
			NewImages: st.newImages,
			NewVideos: st.newVideos,
			FileNames: st.fileNames,
		})
		if err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - events.Insert %q: %w", name, err)
		}
		// Attach the in-memory media after persistence (it is never stored).
		saved.NewMedia = st.newMedia
		report.Events = append(report.Events, saved)
	}

	return report, nil
}

// updateCover queries the DB for a cover image in the album and updates the album record.
func (uc *UseCase) updateCover(ctx context.Context, albumID int) error {
	cover, found, err := uc.images.FindCoverByAlbum(ctx, albumID)
	if err != nil {
		return fmt.Errorf("SyncUseCase - updateCover - FindCoverByAlbum albumID=%d: %w", albumID, err)
	}

	if found {
		if err = uc.albums.SetCover(ctx, albumID, cover.ID); err != nil {
			return fmt.Errorf("SyncUseCase - updateCover - SetCover albumID=%d: %w", albumID, err)
		}
	} else {
		if err = uc.albums.ClearCover(ctx, albumID); err != nil {
			return fmt.Errorf("SyncUseCase - updateCover - ClearCover albumID=%d: %w", albumID, err)
		}
	}
	return nil
}
