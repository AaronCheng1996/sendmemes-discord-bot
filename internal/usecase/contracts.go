// Package usecase implements application business logic. Each logic group in own file.
package usecase

import (
	"context"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

//go:generate mockgen -source=contracts.go -destination=./mocks_usecase_test.go -package=usecase_test

type (
	Translation interface {
		Translate(context.Context, entity.Translation) (entity.Translation, error)
		History(context.Context) (entity.TranslationHistory, error)
	}

	Images interface {
		// GetImage returns the default (fallback) image.
		GetImage(ctx context.Context) (entity.Image, error)
		// GetRandom returns one random image from all images.
		GetRandom(ctx context.Context) (entity.Image, error)
		// GetAlbumImages returns up to limit images from the named album.
		// If the album has a cover, it is always the first element; the rest are random.
		GetAlbumImages(ctx context.Context, albumName string, limit int) ([]entity.Image, error)
		// GetRandomAlbumImages picks a random album then returns up to limit images from it.
		// If the album has a cover, it is always the first element; the rest are random.
		GetRandomAlbumImages(ctx context.Context, limit int) ([]entity.Image, error)
		// GetFullAlbum returns all non-cover images in the named album ordered by id.
		// The cover (if any) is sent separately by the caller via GetAlbumCover.
		GetFullAlbum(ctx context.Context, albumName string) ([]entity.Image, error)
		// GetAlbumCover returns the cover image for the named album.
		// Returns (image, true, nil) when a cover exists, (zero, false, nil) when it does not.
		GetAlbumCover(ctx context.Context, albumName string) (entity.Image, bool, error)
		// ResolveURL returns a public URL suitable for a Discord embed.
		// For pCloud images it calls GetFileLink; for local paths it prepends HTTP_PUBLIC_URL.
		ResolveURL(ctx context.Context, img entity.Image) (string, error)
	}

	Sync interface {
		// SyncImages fetches the pCloud folder tree and reconciles it with the database.
		SyncImages(ctx context.Context) error
	}
)
