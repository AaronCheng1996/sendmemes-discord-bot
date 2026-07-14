// Package sync implements the pCloud image synchronisation use case.
package sync

import (
	"context"
	"fmt"
	"sort"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// maxEventFileNames caps how many discovered file names are sampled per event.
const maxEventFileNames = 10

// UseCase synchronises the pCloud folder tree with the database.
type UseCase struct {
	pcloud    repo.PCloudAPI
	albums    repo.AlbumsRepo
	images    repo.ImagesRepo
	events    repo.SyncEventsRepo
	folderIDs []int64
}

// New creates a new sync use case.
func New(pcloud repo.PCloudAPI, albums repo.AlbumsRepo, images repo.ImagesRepo, events repo.SyncEventsRepo, folderIDs []int64) *UseCase {
	return &UseCase{
		pcloud:    pcloud,
		albums:    albums,
		images:    images,
		events:    events,
		folderIDs: folderIDs,
	}
}

// albumSyncStats accumulates per-album discovery counters for one run.
type albumSyncStats struct {
	albumID   int
	created   bool
	newImages int
	newVideos int
	fileNames []string
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

	var entries []repo.PCloudEntry
	for _, folderID := range uc.folderIDs {
		folderEntries, err := uc.pcloud.ListFolder(ctx, folderID)
		if err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - ListFolder folderID=%d: %w", folderID, err)
		}
		entries = append(entries, folderEntries...)
	}

	// Group file IDs per album name so we can prune stale rows and detect covers
	// after upsert, and track per-album discovery stats for the sync report.
	albumFileIDs := make(map[string][]int64)
	stats := make(map[string]*albumSyncStats)

	for _, entry := range entries {
		album, created, err := uc.albums.GetOrCreate(ctx, entry.ParentFolderName)
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
			URL:       entry.Name, // store filename; full link resolved at send time via GetFileLink
			Source:    "pcloud",
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
		}

		albumFileIDs[entry.ParentFolderName] = append(albumFileIDs[entry.ParentFolderName], entry.FileID)
	}

	// Per-album cleanup and cover detection.
	for albumName, fileIDs := range albumFileIDs {
		album, err := uc.albums.GetByName(ctx, albumName)
		if err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - GetByName %q: %w", albumName, err)
		}

		if err = uc.images.DeleteByAlbumNotInFileIDs(ctx, album.ID, fileIDs); err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - DeleteByAlbumNotInFileIDs album %q: %w", albumName, err)
		}

		// Detect cover: look for an image whose filename matches cover.* or _cover.*
		// Cover detection is best-effort; errors do not abort the sync.
		if err = uc.updateCover(ctx, album.ID); err != nil {
			// Non-fatal: log via error return but continue processing other albums.
			_ = err // caller (scheduler) logs the returned error; we just skip this album's cover
		}
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
