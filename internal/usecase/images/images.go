// Package images implements the image retrieval and URL resolution use case.
package images

import (
	"context"
	"fmt"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// UseCase handles image queries and URL resolution.
type UseCase struct {
	repo      repo.ImagesRepo
	albums    repo.AlbumsRepo
	pcloud    repo.PCloudAPI
	publicURL string // HTTP_PUBLIC_URL, used for local/default images
}

// New creates an images use case.
func New(r repo.ImagesRepo, albums repo.AlbumsRepo, pcloud repo.PCloudAPI, publicURL string) *UseCase {
	return &UseCase{
		repo:      r,
		albums:    albums,
		pcloud:    pcloud,
		publicURL: publicURL,
	}
}

// GetImage returns the default (fallback) image.
func (uc *UseCase) GetImage(ctx context.Context) (entity.Image, error) {
	i, err := uc.repo.GetDefault(ctx)
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesUseCase - GetImage - repo.GetDefault: %w", err)
	}
	return i, nil
}

// GetRandom returns one random image from all images.
func (uc *UseCase) GetRandom(ctx context.Context) (entity.Image, error) {
	i, err := uc.repo.GetRandom(ctx)
	if err != nil {
		return entity.Image{}, fmt.Errorf("ImagesUseCase - GetRandom - repo.GetRandom: %w", err)
	}
	return i, nil
}

// GetAlbumImages returns up to limit images from the named album.
// If the album has a cover, it is always prepended as the first element;
// the remaining slots are filled with random images (excluding the cover).
func (uc *UseCase) GetAlbumImages(ctx context.Context, albumName string, limit int) ([]entity.Image, error) {
	album, err := uc.albums.GetByName(ctx, albumName)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - GetAlbumImages - GetByName %q: %w", albumName, err)
	}
	return uc.albumImagesWithCover(ctx, album, limit)
}

// GetRandomAlbumImages picks a random album and returns up to limit images from it.
// If the album has a cover, it is always prepended as the first element.
func (uc *UseCase) GetRandomAlbumImages(ctx context.Context, limit int) ([]entity.Image, error) {
	album, err := uc.albums.GetRandom(ctx)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - GetRandomAlbumImages - GetRandom: %w", err)
	}
	return uc.albumImagesWithCover(ctx, album, limit)
}

// albumImagesWithCover is the shared logic for GetAlbumImages and GetRandomAlbumImages.
// When the album has a cover: cover is first, then up to (limit-1) random non-cover images.
// When no cover: up to limit random images.
func (uc *UseCase) albumImagesWithCover(ctx context.Context, album entity.Album, limit int) ([]entity.Image, error) {
	if !album.HasCover {
		return uc.repo.GetRandomByAlbum(ctx, album.ID, limit, 0)
	}

	cover, found, err := uc.repo.FindCoverByAlbum(ctx, album.ID)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - albumImagesWithCover - FindCoverByAlbum: %w", err)
	}
	if !found {
		// Cover flag set but image not found; fall back to plain random.
		return uc.repo.GetRandomByAlbum(ctx, album.ID, limit, 0)
	}

	cover.IsCover = true
	rest, err := uc.repo.GetRandomByAlbum(ctx, album.ID, limit-1, cover.ID)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - albumImagesWithCover - GetRandomByAlbum: %w", err)
	}
	return append([]entity.Image{cover}, rest...), nil
}

// GetFullAlbum returns all non-cover images in the named album ordered by id.
// The cover (if any) is excluded here and sent separately by the caller via GetAlbumCover.
func (uc *UseCase) GetFullAlbum(ctx context.Context, albumName string) ([]entity.Image, error) {
	album, err := uc.albums.GetByName(ctx, albumName)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - GetFullAlbum - GetByName %q: %w", albumName, err)
	}

	excludeID := 0
	if album.HasCover {
		cover, found, err := uc.repo.FindCoverByAlbum(ctx, album.ID)
		if err != nil {
			return nil, fmt.Errorf("ImagesUseCase - GetFullAlbum - FindCoverByAlbum: %w", err)
		}
		if found {
			excludeID = cover.ID
		}
	}

	imgs, err := uc.repo.GetAllByAlbum(ctx, album.ID, excludeID)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - GetFullAlbum - GetAllByAlbum: %w", err)
	}
	return imgs, nil
}

// GetAlbumCover returns the cover image for the named album.
// Returns (image, true, nil) when a cover exists, (zero, false, nil) when it does not.
func (uc *UseCase) GetAlbumCover(ctx context.Context, albumName string) (entity.Image, bool, error) {
	album, err := uc.albums.GetByName(ctx, albumName)
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesUseCase - GetAlbumCover - GetByName %q: %w", albumName, err)
	}
	if !album.HasCover {
		return entity.Image{}, false, nil
	}
	cover, found, err := uc.repo.FindCoverByAlbum(ctx, album.ID)
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesUseCase - GetAlbumCover - FindCoverByAlbum: %w", err)
	}
	return cover, found, nil
}

// ResolveURL returns a public URL suitable for a Discord embed.
// - pCloud images: generates a fresh temporary download link via GetFileLink.
// - Local/relative paths (starting with "/"): prepends HTTP_PUBLIC_URL.
// - Already absolute URLs: returned as-is.
func (uc *UseCase) ResolveURL(ctx context.Context, img entity.Image) (string, error) {
	if img.Source == "pcloud" {
		link, err := uc.pcloud.GetFileLink(ctx, img.FileID)
		if err != nil {
			return "", fmt.Errorf("ImagesUseCase - ResolveURL - GetFileLink fileID=%d: %w", img.FileID, err)
		}
		return link, nil
	}
	if strings.HasPrefix(img.URL, "/") {
		return strings.TrimSuffix(uc.publicURL, "/") + img.URL, nil
	}
	return img.URL, nil
}
