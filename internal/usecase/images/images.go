// Package images implements the image retrieval and URL resolution use case.
package images

import (
	"context"
	"fmt"
	"sort"
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

// GetAlbumByID returns the album with the given id.
func (uc *UseCase) GetAlbumByID(ctx context.Context, id int) (entity.Album, error) {
	album, err := uc.albums.GetByID(ctx, id)
	if err != nil {
		return entity.Album{}, fmt.Errorf("ImagesUseCase - GetAlbumByID - GetByID %d: %w", id, err)
	}
	return album, nil
}

// GetAlbumBatch returns up to limit images for album using cover-first rules
// (cover first when present, then random non-cover images).
func (uc *UseCase) GetAlbumBatch(ctx context.Context, album entity.Album, limit int) ([]entity.Image, error) {
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

// GetScheduledAlbum picks a random album with anti-repeat logic (avoiding the
// excludeN most recently sent albums). Pass the album's ID to MarkAlbumSent after
// the message is sent.
func (uc *UseCase) GetScheduledAlbum(ctx context.Context, excludeN int) (entity.Album, error) {
	album, err := uc.albums.GetRandomExcludeRecent(ctx, excludeN)
	if err != nil {
		return entity.Album{}, fmt.Errorf("ImagesUseCase - GetScheduledAlbum - GetRandomExcludeRecent: %w", err)
	}
	return album, nil
}

// GetComicPages returns the album's images in reading order for comic delivery:
// the cover first (when the album has one), then all remaining images sorted by
// natural filename order (so "2.jpg" precedes "10.jpg").
func (uc *UseCase) GetComicPages(ctx context.Context, album entity.Album) ([]entity.Image, error) {
	excludeID := 0
	var pages []entity.Image
	if album.HasCover {
		cover, found, err := uc.repo.FindCoverByAlbum(ctx, album.ID)
		if err != nil {
			return nil, fmt.Errorf("ImagesUseCase - GetComicPages - FindCoverByAlbum: %w", err)
		}
		if found {
			cover.IsCover = true
			excludeID = cover.ID
			pages = append(pages, cover)
		}
	}
	rest, err := uc.repo.GetAllByAlbum(ctx, album.ID, excludeID)
	if err != nil {
		return nil, fmt.Errorf("ImagesUseCase - GetComicPages - GetAllByAlbum: %w", err)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		return naturalLess(rest[i].URL, rest[j].URL)
	})
	return append(pages, rest...), nil
}

// GetRandomVideo returns one random video from the album.
// Returns (image, true, nil) when a video exists, (zero, false, nil) when none.
func (uc *UseCase) GetRandomVideo(ctx context.Context, albumID int) (entity.Image, bool, error) {
	img, found, err := uc.repo.GetRandomVideoByAlbum(ctx, albumID)
	if err != nil {
		return entity.Image{}, false, fmt.Errorf("ImagesUseCase - GetRandomVideo - GetRandomVideoByAlbum: %w", err)
	}
	return img, found, nil
}

// SetAlbumMode updates the named album's send mode, preserving its name and
// existing send-config JSON, and returns the updated album. An empty stored
// config JSON is normalized to "{}" so the persistence layer's ?::jsonb cast
// does not fail.
func (uc *UseCase) SetAlbumMode(ctx context.Context, albumName string, mode entity.AlbumSendMode) (entity.Album, error) {
	album, err := uc.albums.GetByName(ctx, albumName)
	if err != nil {
		return entity.Album{}, fmt.Errorf("ImagesUseCase - SetAlbumMode - GetByName %q: %w", albumName, err)
	}
	configJSON := strings.TrimSpace(album.SendConfigJSON)
	if configJSON == "" {
		configJSON = "{}"
	}
	updated, err := uc.albums.Update(ctx, album.ID, album.Name, mode, configJSON)
	if err != nil {
		return entity.Album{}, fmt.Errorf("ImagesUseCase - SetAlbumMode - Update %q: %w", albumName, err)
	}
	return updated, nil
}

// MarkAlbumSent stamps last_sent_at = NOW() for albumID.
func (uc *UseCase) MarkAlbumSent(ctx context.Context, albumID int) error {
	if err := uc.albums.MarkSent(ctx, albumID); err != nil {
		return fmt.Errorf("ImagesUseCase - MarkAlbumSent: %w", err)
	}
	return nil
}

// IncrAlbumRating increments positive_rating by 1 for albumID.
func (uc *UseCase) IncrAlbumRating(ctx context.Context, albumID int) error {
	if err := uc.albums.IncrRating(ctx, albumID); err != nil {
		return fmt.Errorf("ImagesUseCase - IncrAlbumRating: %w", err)
	}
	return nil
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

// ResolvePublicURL returns a permanent pCloud public share URL for img.
// When img already carries a stored PublicLink it is returned directly (no API
// call). Otherwise a public link is created via the pCloud API, persisted so
// future lookups skip the call, and returned. Public links are not IP-bound and
// never expire, unlike the temporary links from ResolveURL. Non-pCloud images
// fall back to ResolveURL.
func (uc *UseCase) ResolvePublicURL(ctx context.Context, img entity.Image) (string, error) {
	if img.Source != "pcloud" {
		return uc.ResolveURL(ctx, img)
	}
	if img.PublicLink != "" {
		return img.PublicLink, nil
	}
	link, err := uc.pcloud.GetFilePublicLink(ctx, img.FileID)
	if err != nil {
		return "", fmt.Errorf("ImagesUseCase - ResolvePublicURL - GetFilePublicLink fileID=%d: %w", img.FileID, err)
	}
	if err := uc.repo.SetPublicLink(ctx, img.ID, link); err != nil {
		return "", fmt.Errorf("ImagesUseCase - ResolvePublicURL - SetPublicLink id=%d: %w", img.ID, err)
	}
	return link, nil
}

// ResolvePreviewURL returns a URL a browser can render directly in an <img> tag.
//
// ResolveURL's temporary getfilelink URLs are bound to the *bot container's* IP,
// so the dashboard running on someone else's machine renders a broken image —
// the same problem R1 fixed for Discord video links. Public share links are not
// IP-bound but point at a landing page rather than the file, so pCloud images
// resolve to a getpubthumb thumbnail built from the (persisted) share link.
// Non-pCloud images, and pCloud links with no extractable share code, fall back
// to ResolveURL.
func (uc *UseCase) ResolvePreviewURL(ctx context.Context, img entity.Image) (string, error) {
	if img.Source != "pcloud" {
		return uc.ResolveURL(ctx, img)
	}
	link, err := uc.ResolvePublicURL(ctx, img)
	if err != nil {
		return "", fmt.Errorf("ImagesUseCase - ResolvePreviewURL - ResolvePublicURL id=%d: %w", img.ID, err)
	}
	if thumb := uc.pcloud.PublicThumbURL(link, img.FileID, ""); thumb != "" {
		return thumb, nil
	}
	return uc.ResolveURL(ctx, img)
}
