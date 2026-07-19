package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/images"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func imagesUseCase(t *testing.T) (*images.UseCase, *MockImagesRepo, *MockPCloudAPI) {
	t.Helper()

	mockCtl := gomock.NewController(t)
	t.Cleanup(mockCtl.Finish)

	repoMock := NewMockImagesRepo(mockCtl)
	albums := NewMockAlbumsRepo(mockCtl)
	pcloud := NewMockPCloudAPI(mockCtl)

	uc := images.New(repoMock, albums, pcloud, "https://example.test")

	return uc, repoMock, pcloud
}

// A stored PublicLink is returned directly without hitting the pCloud API or
// persisting again.
func TestResolvePublicURLUsesStoredLink(t *testing.T) {
	t.Parallel()

	uc, _, _ := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{
		ID:         7,
		Source:     "pcloud",
		FileID:     42,
		PublicLink: "https://u.pcloud.link/publink/show?code=cached",
	}

	url, err := uc.ResolvePublicURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, img.PublicLink, url)
}

// On first resolution the link is fetched from pCloud and persisted.
func TestResolvePublicURLResolvesAndPersists(t *testing.T) {
	t.Parallel()

	uc, repoMock, pcloud := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{ID: 7, Source: "pcloud", FileID: 42}
	link := "https://u.pcloud.link/publink/show?code=fresh"

	pcloud.EXPECT().GetFilePublicLink(ctx, int64(42)).Return(link, nil)
	repoMock.EXPECT().SetPublicLink(ctx, 7, link).Return(nil)

	url, err := uc.ResolvePublicURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, link, url)
}

// A pCloud API failure is surfaced and nothing is persisted.
func TestResolvePublicURLAPIError(t *testing.T) {
	t.Parallel()

	uc, _, pcloud := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{ID: 7, Source: "pcloud", FileID: 42}

	pcloud.EXPECT().GetFilePublicLink(ctx, int64(42)).Return("", errors.New("boom"))

	_, err := uc.ResolvePublicURL(ctx, img)
	require.Error(t, err)
}

// Non-pCloud images fall back to ResolveURL (local path → HTTP_PUBLIC_URL).
func TestResolvePublicURLNonPCloudFallback(t *testing.T) {
	t.Parallel()

	uc, _, _ := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{ID: 8, Source: "local", URL: "/media/x.png"}

	url, err := uc.ResolvePublicURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, "https://example.test/media/x.png", url)
}

// A pCloud preview is the getpubthumb URL built from the stored share link, not
// the landing-page link itself.
func TestResolvePreviewURLUsesPublicThumb(t *testing.T) {
	t.Parallel()

	uc, _, pcloud := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{
		ID:         7,
		Source:     "pcloud",
		FileID:     42,
		PublicLink: "https://u.pcloud.link/publink/show?code=cached",
	}
	thumb := "https://api.pcloud.com/getpubthumb?code=cached&fileid=42&size=512x512"

	pcloud.EXPECT().PublicThumbURL(img.PublicLink, int64(42), "").Return(thumb)

	url, err := uc.ResolvePreviewURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, thumb, url)
}

// When no share code can be extracted the preview falls back to a temporary
// getfilelink URL rather than returning the unusable landing page.
func TestResolvePreviewURLFallsBackWhenNoThumb(t *testing.T) {
	t.Parallel()

	uc, _, pcloud := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{
		ID:         7,
		Source:     "pcloud",
		FileID:     42,
		PublicLink: "https://u.pcloud.link/publink/show",
	}

	pcloud.EXPECT().PublicThumbURL(img.PublicLink, int64(42), "").Return("")
	pcloud.EXPECT().GetFileLink(ctx, int64(42)).Return("https://p-def1.pcloud.com/temp.png", nil)

	url, err := uc.ResolvePreviewURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, "https://p-def1.pcloud.com/temp.png", url)
}

// A first-time pCloud preview resolves and persists the share link before
// building the thumbnail URL.
func TestResolvePreviewURLResolvesLinkFirst(t *testing.T) {
	t.Parallel()

	uc, repoMock, pcloud := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{ID: 7, Source: "pcloud", FileID: 42}
	link := "https://u.pcloud.link/publink/show?code=fresh"
	thumb := "https://api.pcloud.com/getpubthumb?code=fresh&fileid=42&size=512x512"

	pcloud.EXPECT().GetFilePublicLink(ctx, int64(42)).Return(link, nil)
	repoMock.EXPECT().SetPublicLink(ctx, 7, link).Return(nil)
	pcloud.EXPECT().PublicThumbURL(link, int64(42), "").Return(thumb)

	url, err := uc.ResolvePreviewURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, thumb, url)
}

// Non-pCloud images keep using ResolveURL.
func TestResolvePreviewURLNonPCloudFallback(t *testing.T) {
	t.Parallel()

	uc, _, _ := imagesUseCase(t)
	ctx := context.Background()

	img := entity.Image{ID: 8, Source: "local", URL: "/media/x.png"}

	url, err := uc.ResolvePreviewURL(ctx, img)
	require.NoError(t, err)
	require.Equal(t, "https://example.test/media/x.png", url)
}
