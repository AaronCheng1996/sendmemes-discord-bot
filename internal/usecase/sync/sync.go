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

// albumGroup collects the files one walked folder contributed, so an album is
// resolved once per run rather than once per file. Folders are keyed by name
// (the album's identity); folderID is the id of the first file's parent, which
// only differs between files when two folders share a name and therefore merge
// into one album anyway.
type albumGroup struct {
	name     string
	folderID int64
	entries  []repo.MediaEntry
}

// albumSyncStats accumulates one album's counters for one run: what the walk
// added, and what it took away.
type albumSyncStats struct {
	albumID     int
	created     bool
	renamedFrom string
	// vanished marks an album whose folder was not in this walk at all, as
	// opposed to one that merely lost some files.
	vanished      bool
	newImages     int
	newVideos     int
	fileNames     []string
	newMedia      []entity.Image
	removedImages int
	removedVideos int
	removedNames  []string
}

// SyncImages fetches the full source folder tree and reconciles it with the database:
//  1. Group the walked files by folder, then resolve each folder to its album
//     (following a rename by folder id) and upsert every file row.
//  2. Soft-delete the rows for files the folder no longer holds.
//  3. Detect cover images (filename matches cover.* or _cover.*) and update album.has_cover.
//  4. Flag the albums whose folder disappeared and soft-delete their files too.
//  5. Record one sync event per album that changed. Events describing new
//     content go to report.Events (deliverable to Discord); removals and renames
//     go to report.Notices, which is for the activity log only. InitialImport is
//     set when the database had no albums before this run, so callers can
//     suppress notifications on first import.
//
// Nothing here deletes a row. Both an album and a file are retired by a flag, so
// a folder that is moved away and comes back recovers on its own.
func (uc *UseCase) SyncImages(ctx context.Context) (entity.SyncReport, error) {
	// Missing albums count too: the question is whether this database has ever
	// been populated, not whether its folders are all still there.
	priorAlbums, err := uc.albums.Count(ctx, repo.AlbumAdminListQuery{IncludeMissing: true})
	if err != nil {
		return entity.SyncReport{}, fmt.Errorf("SyncUseCase - SyncImages - albums.Count: %w", err)
	}
	report := entity.SyncReport{InitialImport: priorAlbums == 0}

	entries, err := uc.source.ListMedia(ctx)
	if err != nil {
		return report, fmt.Errorf("SyncUseCase - SyncImages - ListMedia: %w", err)
	}

	groups := groupByFolder(entries)
	stats := make(map[string]*albumSyncStats, len(groups))
	seen := make([]string, 0, len(groups))

	for _, group := range groups {
		album, res, rerr := uc.albums.ResolveByFolder(ctx, group.folderID, group.name, uc.defaultMode)
		if rerr != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - ResolveByFolder %q: %w", group.name, rerr)
		}
		// A rename has already moved the row to the folder's name, so from here
		// on the album is only known by the name the walk reported.
		st := &albumSyncStats{albumID: album.ID, created: res.Created, renamedFrom: res.RenamedFrom}
		stats[group.name] = st
		seen = append(seen, group.name)

		fileIDs := make([]int64, 0, len(group.entries))
		for _, entry := range group.entries {
			fileIDs = append(fileIDs, entry.FileID)
			if uerr := uc.upsertEntry(ctx, album.ID, group.name, entry, st); uerr != nil {
				return report, uerr
			}
		}

		removed, derr := uc.images.SoftDeleteByAlbumNotInFileIDs(ctx, album.ID, uc.sourceName, fileIDs)
		if derr != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - SoftDeleteByAlbumNotInFileIDs album %q: %w", group.name, derr)
		}
		st.countRemoved(removed)

		// Detect cover: look for an image whose filename matches cover.* or _cover.*
		// Cover detection is best-effort; errors do not abort the sync.
		if cerr := uc.updateCover(ctx, album.ID); cerr != nil {
			// Non-fatal: log via error return but continue processing other albums.
			_ = cerr // caller (scheduler) logs the returned error; we just skip this album's cover
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
		if err = uc.albums.ClearMissing(ctx, seen); err != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - ClearMissing: %w", err)
		}
		marked, merr := uc.albums.MarkMissingExcept(ctx, seen)
		if merr != nil {
			return report, fmt.Errorf("SyncUseCase - SyncImages - MarkMissingExcept: %w", merr)
		}
		sort.Slice(marked, func(i, j int) bool { return marked[i].Name < marked[j].Name })

		for _, album := range marked {
			report.MissingAlbums = append(report.MissingAlbums, album.Name)

			// The folder is gone, so its files are as unreachable as the ones
			// pruned out of a folder that survived: retire them the same way.
			removed, rerr := uc.images.SoftDeleteByAlbum(ctx, album.ID, uc.sourceName)
			if rerr != nil {
				return report, fmt.Errorf("SyncUseCase - SyncImages - SoftDeleteByAlbum %q: %w", album.Name, rerr)
			}
			st := &albumSyncStats{albumID: album.ID, vanished: true}
			st.countRemoved(removed)
			stats[album.Name] = st
		}
	}

	if rerr := uc.recordEvents(ctx, &report, stats); rerr != nil {
		return report, rerr
	}
	return report, nil
}

// groupByFolder buckets a walk's files by their album name, preserving the order
// the source reported so the run's events stay stable between syncs. Two folders
// sharing a name still merge into one album; the first one's id wins.
func groupByFolder(entries []repo.MediaEntry) []*albumGroup {
	groups := make([]*albumGroup, 0, len(entries))
	index := make(map[string]*albumGroup, len(entries))
	for _, entry := range entries {
		g := index[entry.ParentFolderName]
		if g == nil {
			g = &albumGroup{name: entry.ParentFolderName, folderID: entry.ParentFolderID}
			index[entry.ParentFolderName] = g
			groups = append(groups, g)
		}
		g.entries = append(g.entries, entry)
	}
	return groups
}

// upsertEntry writes one walked file into albumID and records it as new content
// when the row did not already exist (a revived soft-deleted row does not count).
func (uc *UseCase) upsertEntry(ctx context.Context, albumID int, albumName string, entry repo.MediaEntry, st *albumSyncStats) error {
	inserted, err := uc.images.UpsertByFileID(ctx, entity.Image{
		FileID:    entry.FileID,
		URL:       entry.Name, // store filename; full link resolved at send time via the MediaSource
		Source:    uc.sourceName,
		AlbumID:   albumID,
		Kind:      entry.Kind,
		SizeBytes: entry.Size,
	})
	if err != nil {
		return fmt.Errorf("SyncUseCase - SyncImages - UpsertByFileID fileID=%d: %w", entry.FileID, err)
	}
	if !inserted {
		return nil
	}

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
			AlbumID:   albumID,
			AlbumName: albumName, // no DB round-trip yet, so carry it from the walk
			Kind:      entry.Kind,
			SizeBytes: entry.Size,
		})
	}
	return nil
}

// countRemoved folds a soft-delete result into the album's counters.
func (st *albumSyncStats) countRemoved(removed []entity.Image) {
	for i := range removed {
		img := &removed[i]
		if img.Kind == entity.MediaKindVideo {
			st.removedVideos++
		} else {
			st.removedImages++
		}
		if len(st.removedNames) < maxEventFileNames {
			st.removedNames = append(st.removedNames, img.URL)
		}
	}
}

// recordEvents persists this run's events in album name order, so the output is
// deterministic. Each saved event is filed by whether its type maps to a
// delivery-rule trigger: those go to report.Events and may reach Discord, the
// rest to report.Notices, which the activity log reads and the notifier never
// touches.
func (uc *UseCase) recordEvents(ctx context.Context, report *entity.SyncReport, stats map[string]*albumSyncStats) error {
	names := make([]string, 0, len(stats))
	for name := range stats {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		st := stats[name]
		pending := pendingEvents(name, st)
		for i := range pending {
			saved, err := uc.events.Insert(ctx, pending[i])
			if err != nil {
				return fmt.Errorf("SyncUseCase - SyncImages - events.Insert %s %q: %w", pending[i].EventType, name, err)
			}
			if entity.SyncEventTriggerType(saved.EventType) == "" {
				report.Notices = append(report.Notices, saved)
				continue
			}
			// Attach the in-memory media after persistence (it is never stored).
			saved.NewMedia = st.newMedia
			report.Events = append(report.Events, saved)
		}
	}

	return nil
}

// pendingEvents turns one album's counters into the events they call for, in a
// stable order: the rename first, then what the run added, then what it took
// away. An album can produce all three — a renamed folder may have gained files
// and lost others in the same run.
func pendingEvents(name string, st *albumSyncStats) []entity.SyncEvent {
	var events []entity.SyncEvent

	if st.renamedFrom != "" {
		events = append(events, entity.SyncEvent{
			EventType:    entity.SyncEventAlbumRenamed,
			AlbumID:      st.albumID,
			AlbumName:    name,
			PreviousName: st.renamedFrom,
		})
	}

	if st.created || st.newImages > 0 || st.newVideos > 0 {
		eventType := entity.SyncEventFilesAdded
		if st.created {
			eventType = entity.SyncEventAlbumCreated
		}
		events = append(events, entity.SyncEvent{
			EventType: eventType,
			AlbumID:   st.albumID,
			AlbumName: name,
			NewImages: st.newImages,
			NewVideos: st.newVideos,
			FileNames: st.fileNames,
		})
	}

	// A vanished album is worth an event even when it held no files; a folder
	// that survived only reports what it actually lost.
	if st.vanished || st.removedImages > 0 || st.removedVideos > 0 {
		eventType := entity.SyncEventFilesRemoved
		if st.vanished {
			eventType = entity.SyncEventAlbumMissing
		}
		events = append(events, entity.SyncEvent{
			EventType:     eventType,
			AlbumID:       st.albumID,
			AlbumName:     name,
			RemovedImages: st.removedImages,
			RemovedVideos: st.removedVideos,
			FileNames:     st.removedNames,
		})
	}

	return events
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
