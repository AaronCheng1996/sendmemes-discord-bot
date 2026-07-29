package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	syncuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/sync"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testDefaultSendMode is the configured default send mode threaded through the
// sync use case into GetOrCreate; a non-Random value proves it is passed through.
const testDefaultSendMode = entity.AlbumSendModeSingle

// testSourceName is the source label the test use case is constructed with;
// asserted against on every upsert/prune call to prove it is passed through.
const testSourceName = entity.MediaSourcePCloud

func syncUseCase(t *testing.T) (*syncuc.UseCase, *MockMediaSource, *MockAlbumsRepo, *MockImagesRepo, *MockSyncEventsRepo) {
	t.Helper()

	mockCtl := gomock.NewController(t)
	t.Cleanup(mockCtl.Finish)

	source := NewMockMediaSource(mockCtl)
	albums := NewMockAlbumsRepo(mockCtl)
	images := NewMockImagesRepo(mockCtl)
	events := NewMockSyncEventsRepo(mockCtl)

	useCase := syncuc.New(source, testSourceName, albums, images, events, testDefaultSendMode)

	return useCase, source, albums, images, events
}

// noCoverCleanup registers the expectations for the per-album cleanup pass of
// an album without a cover image.
func noCoverCleanup(ctx context.Context, albums *MockAlbumsRepo, images *MockImagesRepo, album entity.Album, fileIDs []int64) {
	albums.EXPECT().GetByName(ctx, album.Name).Return(album, nil)
	images.EXPECT().DeleteByAlbumNotInFileIDs(ctx, album.ID, testSourceName, fileIDs).Return(nil)
	images.EXPECT().FindCoverByAlbum(ctx, album.ID).Return(entity.Image{}, false, nil)
	albums.EXPECT().ClearCover(ctx, album.ID).Return(nil)
}

// expectMissingPass registers the missing-flag reconciliation that every
// non-empty sync run performs. Album order is map-driven, so names are matched
// loosely and asserted separately where it matters.
func expectMissingPass(albums *MockAlbumsRepo, marked []string) {
	albums.EXPECT().ClearMissing(gomock.Any(), gomock.Any()).Return(nil)
	albums.EXPECT().MarkMissingExcept(gomock.Any(), gomock.Any()).Return(marked, nil)
}

func TestSyncImagesReportsDiscoveries(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	albumA := entity.Album{ID: 1, Name: "AlbumA"}
	albumB := entity.Album{ID: 2, Name: "AlbumB"}

	albums.EXPECT().Count(ctx, repo.AlbumAdminListQuery{}).Return(1, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 11, Name: "1.jpg", ParentFolderName: "AlbumA", Kind: entity.MediaKindImage, Size: 100},
		{FileID: 12, Name: "clip.mp4", ParentFolderName: "AlbumA", Kind: entity.MediaKindVideo, Size: 2000},
		{FileID: 21, Name: "old.jpg", ParentFolderName: "AlbumB", Kind: entity.MediaKindImage, Size: 50},
	}, nil)

	// AlbumA is created on first sight, then updated on the second file.
	albums.EXPECT().GetOrCreate(ctx, "AlbumA", testDefaultSendMode).Return(albumA, true, nil)
	albums.EXPECT().GetOrCreate(ctx, "AlbumA", testDefaultSendMode).Return(albumA, false, nil)
	albums.EXPECT().GetOrCreate(ctx, "AlbumB", testDefaultSendMode).Return(albumB, false, nil)

	images.EXPECT().UpsertByFileID(ctx, entity.Image{
		FileID: 11, URL: "1.jpg", Source: "pcloud", AlbumID: 1, Kind: entity.MediaKindImage, SizeBytes: 100,
	}).Return(true, nil)
	images.EXPECT().UpsertByFileID(ctx, entity.Image{
		FileID: 12, URL: "clip.mp4", Source: "pcloud", AlbumID: 1, Kind: entity.MediaKindVideo, SizeBytes: 2000,
	}).Return(true, nil)
	images.EXPECT().UpsertByFileID(ctx, entity.Image{
		FileID: 21, URL: "old.jpg", Source: "pcloud", AlbumID: 2, Kind: entity.MediaKindImage, SizeBytes: 50,
	}).Return(false, nil)

	noCoverCleanup(ctx, albums, images, albumA, []int64{11, 12})
	noCoverCleanup(ctx, albums, images, albumB, []int64{21})

	now := time.Now()
	events.EXPECT().Insert(ctx, entity.SyncEvent{
		EventType: entity.SyncEventAlbumCreated,
		AlbumID:   1,
		AlbumName: "AlbumA",
		NewImages: 1,
		NewVideos: 1,
		FileNames: []string{"1.jpg", "clip.mp4"},
	}).DoAndReturn(func(_ context.Context, ev entity.SyncEvent) (entity.SyncEvent, error) {
		ev.ID = 7
		ev.CreatedAt = now
		return ev, nil
	})

	expectMissingPass(albums, nil)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.False(t, report.InitialImport)
	require.Len(t, report.Events, 1)
	require.Equal(t, int64(7), report.Events[0].ID)
	require.Equal(t, entity.SyncEventAlbumCreated, report.Events[0].EventType)
	require.Equal(t, 1, report.Events[0].NewImages)
	require.Equal(t, 1, report.Events[0].NewVideos)
	// The in-memory report carries the new media records for the notifier.
	require.Len(t, report.Events[0].NewMedia, 2)
}

func TestSyncImagesInitialImport(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 1, Name: "First"}

	albums.EXPECT().Count(ctx, repo.AlbumAdminListQuery{}).Return(0, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 11, Name: "a.jpg", ParentFolderName: "First", Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().GetOrCreate(ctx, "First", testDefaultSendMode).Return(album, true, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(true, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{11})

	// Events are still recorded on initial import; only Discord delivery is
	// suppressed (by the caller, based on report.InitialImport).
	events.EXPECT().Insert(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, ev entity.SyncEvent) (entity.SyncEvent, error) {
			ev.ID = 1
			return ev, nil
		})

	expectMissingPass(albums, nil)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.True(t, report.InitialImport)
	require.Len(t, report.Events, 1)
}

func TestSyncImagesNoNewContent(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 3, Name: "Stable"}

	albums.EXPECT().Count(ctx, repo.AlbumAdminListQuery{}).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 31, Name: "same.jpg", ParentFolderName: "Stable", Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().GetOrCreate(ctx, "Stable", testDefaultSendMode).Return(album, false, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{31})

	// No events.Insert expectation: nothing new was discovered.
	_ = events

	expectMissingPass(albums, nil)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.False(t, report.InitialImport)
	require.Empty(t, report.Events)
}

func TestSyncImagesMarksVanishedAlbum(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 1, Name: "Kept"}

	albums.EXPECT().Count(ctx, repo.AlbumAdminListQuery{}).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 11, Name: "a.jpg", ParentFolderName: "Kept", Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().GetOrCreate(ctx, "Kept", testDefaultSendMode).Return(album, false, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{11})
	_ = events

	// Only "Kept" was seen, so the album whose folder disappeared is flagged
	// (and reported) rather than deleted.
	albums.EXPECT().ClearMissing(ctx, []string{"Kept"}).Return(nil)
	albums.EXPECT().MarkMissingExcept(ctx, []string{"Kept"}).Return([]string{"Gone"}, nil)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.Equal(t, []string{"Gone"}, report.MissingAlbums)
	require.False(t, report.EmptyScan)
}

func TestSyncImagesClearsMissingWhenFolderReturns(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	// The album is still flagged missing in the DB; seeing it again must clear it.
	flagged := time.Now().Add(-24 * time.Hour)
	album := entity.Album{ID: 5, Name: "Back", MissingSince: &flagged}

	albums.EXPECT().Count(ctx, repo.AlbumAdminListQuery{}).Return(1, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 51, Name: "b.jpg", ParentFolderName: "Back", Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().GetOrCreate(ctx, "Back", testDefaultSendMode).Return(album, false, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(true, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{51})
	events.EXPECT().Insert(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, ev entity.SyncEvent) (entity.SyncEvent, error) {
			ev.ID = 1
			return ev, nil
		})

	albums.EXPECT().ClearMissing(ctx, []string{"Back"}).Return(nil)
	albums.EXPECT().MarkMissingExcept(ctx, []string{"Back"}).Return(nil, nil)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.Empty(t, report.MissingAlbums)
}

func TestSyncImagesEmptyScanSkipsMissingPass(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	albums.EXPECT().Count(ctx, repo.AlbumAdminListQuery{}).Return(3, nil)
	// A source that succeeds but returns nothing almost always means a broken
	// configuration, so nothing may be flagged: no ClearMissing/MarkMissingExcept
	// expectations are registered, and the mock controller fails if they are called.
	source.EXPECT().ListMedia(ctx).Return(nil, nil)
	_, _ = images, events

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.True(t, report.EmptyScan)
	require.Empty(t, report.MissingAlbums)
}
