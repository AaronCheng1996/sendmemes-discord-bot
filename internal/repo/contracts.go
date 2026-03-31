// Package repo implements application outer layer logic. Each logic group in own file.
package repo

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

//go:generate mockgen -source=contracts.go -destination=../usecase/mocks_repo_test.go -package=usecase_test

// PCloudEntry is a single image file discovered in the pCloud folder tree.
type PCloudEntry struct {
	FileID           int64
	Name             string
	ParentFolderName string // immediate parent folder name (= album name)
}

type (
	// TranslationRepo -.
	TranslationRepo interface {
		Store(context.Context, entity.Translation) error
		GetHistory(context.Context) ([]entity.Translation, error)
	}

	// TranslationWebAPI -.
	TranslationWebAPI interface {
		Translate(entity.Translation) (entity.Translation, error)
	}

	// AlbumsRepo manages album persistence.
	AlbumsRepo interface {
		List(ctx context.Context, offset, limit int) ([]entity.Album, error)
		GetByID(ctx context.Context, id int) (entity.Album, error)
		Create(ctx context.Context, name string) (entity.Album, error)
		GetOrCreate(ctx context.Context, name string) (entity.Album, error)
		GetByName(ctx context.Context, name string) (entity.Album, error)
		GetRandom(ctx context.Context) (entity.Album, error)
		Update(ctx context.Context, id int, name string) (entity.Album, error)
		Delete(ctx context.Context, id int) error
		// GetRandomExcludeRecent returns a random album that is NOT among the
		// excludeN most recently sent (by last_sent_at DESC).
		// When no eligible album exists (all sent within the history window),
		// it falls back to GetRandom so the scheduler never stalls.
		GetRandomExcludeRecent(ctx context.Context, excludeN int) (entity.Album, error)
		// MarkSent stamps last_sent_at = NOW() for albumID.
		MarkSent(ctx context.Context, albumID int) error
		// IncrRating increments positive_rating by 1 for albumID.
		IncrRating(ctx context.Context, albumID int) error
		// SetCover marks an album as having a cover and records which image it is.
		SetCover(ctx context.Context, albumID, coverImageID int) error
		// ClearCover removes cover designation from an album.
		ClearCover(ctx context.Context, albumID int) error
	}

	// ImagesRepo manages image persistence.
	ImagesRepo interface {
		List(ctx context.Context, albumID, offset, limit int) ([]entity.Image, error)
		GetByID(ctx context.Context, id int) (entity.Image, error)
		GetDefault(ctx context.Context) (entity.Image, error)
		GetRandom(ctx context.Context) (entity.Image, error)
		Insert(ctx context.Context, img entity.Image) (entity.Image, error)
		Update(ctx context.Context, img entity.Image) (entity.Image, error)
		Delete(ctx context.Context, id int) error
		// GetRandomByAlbum returns up to limit random images from albumID,
		// optionally excluding the image with excludeID (pass 0 for no exclusion).
		GetRandomByAlbum(ctx context.Context, albumID, limit, excludeID int) ([]entity.Image, error)
		// GetAllByAlbum returns all images in albumID ordered by id,
		// optionally excluding the image with excludeID (pass 0 for no exclusion).
		GetAllByAlbum(ctx context.Context, albumID, excludeID int) ([]entity.Image, error)
		UpsertByFileID(ctx context.Context, img entity.Image) error
		DeleteByAlbumNotInFileIDs(ctx context.Context, albumID int, fileIDs []int64) error
		// FindCoverByAlbum returns the image in albumID whose filename matches
		// the cover convention (cover.* or _cover.*), case-insensitive.
		FindCoverByAlbum(ctx context.Context, albumID int) (entity.Image, bool, error)
	}

	// ScheduleSettingsRepo manages runtime send scheduling per guild.
	ScheduleSettingsRepo interface {
		GetByGuild(ctx context.Context, guildID string) (entity.DiscordScheduleSettings, bool, error)
		Upsert(ctx context.Context, cfg entity.DiscordScheduleSettings) (entity.DiscordScheduleSettings, error)
	}

	// PCloudAPI abstracts the pCloud REST API.
	PCloudAPI interface {
		ListFolder(ctx context.Context, folderID int64) ([]PCloudEntry, error)
		GetFileLink(ctx context.Context, fileID int64) (string, error)
	}
)
