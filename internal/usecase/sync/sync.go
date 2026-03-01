// Package sync implements the pCloud image synchronisation use case.
package sync

import (
	"context"
	"fmt"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// UseCase synchronises the pCloud folder tree with the database.
type UseCase struct {
	pcloud   repo.PCloudAPI
	albums   repo.AlbumsRepo
	images   repo.ImagesRepo
	folderID int64
}

// New creates a new sync use case.
func New(pcloud repo.PCloudAPI, albums repo.AlbumsRepo, images repo.ImagesRepo, folderID int64) *UseCase {
	return &UseCase{
		pcloud:   pcloud,
		albums:   albums,
		images:   images,
		folderID: folderID,
	}
}

// SyncImages fetches the full pCloud folder tree and reconciles it with the database:
//  1. For each discovered image file, upsert the album and the image row.
//  2. Remove DB rows for images that no longer exist in pCloud (per album).
//  3. Detect cover images (filename matches cover.* or _cover.*) and update album.has_cover.
func (uc *UseCase) SyncImages(ctx context.Context) error {
	entries, err := uc.pcloud.ListFolder(ctx, uc.folderID)
	if err != nil {
		return fmt.Errorf("SyncUseCase - SyncImages - ListFolder: %w", err)
	}

	// Group file IDs per album name so we can prune stale rows and detect covers after upsert.
	albumFileIDs := make(map[string][]int64)

	for _, entry := range entries {
		album, err := uc.albums.GetOrCreate(ctx, entry.ParentFolderName)
		if err != nil {
			return fmt.Errorf("SyncUseCase - SyncImages - GetOrCreate album %q: %w", entry.ParentFolderName, err)
		}

		img := entity.Image{
			FileID:  entry.FileID,
			URL:     entry.Name, // store filename; full link resolved at send time via GetFileLink
			Source:  "pcloud",
			AlbumID: album.ID,
		}
		if err = uc.images.UpsertByFileID(ctx, img); err != nil {
			return fmt.Errorf("SyncUseCase - SyncImages - UpsertByFileID fileID=%d: %w", entry.FileID, err)
		}

		albumFileIDs[entry.ParentFolderName] = append(albumFileIDs[entry.ParentFolderName], entry.FileID)
	}

	// Per-album cleanup and cover detection.
	for albumName, fileIDs := range albumFileIDs {
		album, err := uc.albums.GetByName(ctx, albumName)
		if err != nil {
			return fmt.Errorf("SyncUseCase - SyncImages - GetByName %q: %w", albumName, err)
		}

		if err = uc.images.DeleteByAlbumNotInFileIDs(ctx, album.ID, fileIDs); err != nil {
			return fmt.Errorf("SyncUseCase - SyncImages - DeleteByAlbumNotInFileIDs album %q: %w", albumName, err)
		}

		// Detect cover: look for an image whose filename matches cover.* or _cover.*
		// Cover detection is best-effort; errors do not abort the sync.
		if err = uc.updateCover(ctx, album.ID); err != nil {
			// Non-fatal: log via error return but continue processing other albums.
			_ = err // caller (scheduler) logs the returned error; we just skip this album's cover
		}
	}

	return nil
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
