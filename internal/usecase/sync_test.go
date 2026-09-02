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
// sync use case into ResolveByFolder; a non-Random value proves it is passed through.
const testDefaultSendMode = entity.AlbumSendModeSingle

// testSourceName is the source label the test use case is constructed with;
// asserted against on every upsert/prune call to prove it is passed through.
const testSourceName = entity.MediaSourcePCloud

// countQuery is the album count query the sync runs to decide InitialImport.
// Missing albums are included: the question is whether the DB was ever
// populated, not whether its folders are all still there.
func countQuery() repo.AlbumAdminListQuery {
	return repo.AlbumAdminListQuery{IncludeMissing: true}
}

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

// noCoverCleanup registers the per-album cleanup pass of an album without a
// cover image, whose folder lost nothing this run.
func noCoverCleanup(ctx context.Context, albums *MockAlbumsRepo, images *MockImagesRepo, album entity.Album, fileIDs []int64) {
	images.EXPECT().SoftDeleteByAlbumNotInFileIDs(ctx, album.ID, testSourceName, fileIDs).Return(nil, nil)
	images.EXPECT().FindCoverByAlbum(ctx, album.ID).Return(entity.Image{}, false, nil)
	albums.EXPECT().ClearCover(ctx, album.ID).Return(nil)
}

// expectNothingMissing registers the missing-flag reconciliation of a run where
// every album was seen again. Album order is map-driven, so the arguments are
// matched loosely; the tests that care assert on them directly instead.
func expectNothingMissing(albums *MockAlbumsRepo) {
	albums.EXPECT().ClearMissing(gomock.Any(), gomock.Any()).Return(nil)
	albums.EXPECT().MarkMissingExcept(gomock.Any(), gomock.Any()).Return(nil, nil)
}

// captureEvent records an inserted event and stamps it with an id, mirroring
// what the real repository returns.
func captureEvent(id int64) func(context.Context, entity.SyncEvent) (entity.SyncEvent, error) {
	return func(_ context.Context, ev entity.SyncEvent) (entity.SyncEvent, error) {
		ev.ID = id
		return ev, nil
	}
}

func TestSyncImagesReportsDiscoveries(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	albumA := entity.Album{ID: 1, Name: "AlbumA", FolderID: 100}
	albumB := entity.Album{ID: 2, Name: "AlbumB", FolderID: 200}

	albums.EXPECT().Count(ctx, countQuery()).Return(1, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 11, Name: "1.jpg", ParentFolderName: "AlbumA", ParentFolderID: 100, Kind: entity.MediaKindImage, Size: 100},
		{FileID: 12, Name: "clip.mp4", ParentFolderName: "AlbumA", ParentFolderID: 100, Kind: entity.MediaKindVideo, Size: 2000},
		{FileID: 21, Name: "old.jpg", ParentFolderName: "AlbumB", ParentFolderID: 200, Kind: entity.MediaKindImage, Size: 50},
	}, nil)

	// Each folder is resolved once per run, however many files it holds.
	albums.EXPECT().ResolveByFolder(ctx, int64(100), "AlbumA", testDefaultSendMode).
		Return(albumA, repo.AlbumResolution{Created: true}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(200), "AlbumB", testDefaultSendMode).
		Return(albumB, repo.AlbumResolution{}, nil)

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

	expectNothingMissing(albums)

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
	require.Empty(t, report.Notices)
}

func TestSyncImagesInitialImport(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 1, Name: "First"}

	albums.EXPECT().Count(ctx, countQuery()).Return(0, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 11, Name: "a.jpg", ParentFolderName: "First", ParentFolderID: 100, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(100), "First", testDefaultSendMode).
		Return(album, repo.AlbumResolution{Created: true}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(true, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{11})

	// Events are still recorded on initial import; only Discord delivery is
	// suppressed (by the caller, based on report.InitialImport).
	events.EXPECT().Insert(ctx, gomock.Any()).DoAndReturn(captureEvent(1))

	expectNothingMissing(albums)

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

	albums.EXPECT().Count(ctx, countQuery()).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 31, Name: "same.jpg", ParentFolderName: "Stable", ParentFolderID: 300, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(300), "Stable", testDefaultSendMode).
		Return(album, repo.AlbumResolution{}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{31})

	// No events.Insert expectation: nothing changed in either direction.
	_ = events

	expectNothingMissing(albums)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.False(t, report.InitialImport)
	require.Empty(t, report.Events)
	require.Empty(t, report.Notices)
}

// A file that was soft-deleted and comes back is revived by the upsert, so it is
// reported as an update rather than an insert and must not be announced again.
func TestSyncImagesRevivedFileIsNotNewContent(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 4, Name: "Revived"}

	albums.EXPECT().Count(ctx, countQuery()).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 41, Name: "back.jpg", ParentFolderName: "Revived", ParentFolderID: 400, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(400), "Revived", testDefaultSendMode).
		Return(album, repo.AlbumResolution{}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{41})
	_ = events

	expectNothingMissing(albums)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.Empty(t, report.Events)
}

// A folder that survived but lost files records a files_removed notice, and that
// notice stays out of report.Events so nothing is posted to Discord.
func TestSyncImagesRecordsRemovedFiles(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 5, Name: "Trimmed", FolderID: 500}

	albums.EXPECT().Count(ctx, countQuery()).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 51, Name: "kept.jpg", ParentFolderName: "Trimmed", ParentFolderID: 500, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(500), "Trimmed", testDefaultSendMode).
		Return(album, repo.AlbumResolution{}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)

	images.EXPECT().SoftDeleteByAlbumNotInFileIDs(ctx, 5, testSourceName, []int64{51}).Return([]entity.Image{
		{ID: 91, URL: "gone.jpg", Kind: entity.MediaKindImage},
		{ID: 92, URL: "gone.mp4", Kind: entity.MediaKindVideo},
	}, nil)
	images.EXPECT().FindCoverByAlbum(ctx, 5).Return(entity.Image{}, false, nil)
	albums.EXPECT().ClearCover(ctx, 5).Return(nil)

	events.EXPECT().Insert(ctx, entity.SyncEvent{
		EventType:     entity.SyncEventFilesRemoved,
		AlbumID:       5,
		AlbumName:     "Trimmed",
		RemovedImages: 1,
		RemovedVideos: 1,
		FileNames:     []string{"gone.jpg", "gone.mp4"},
	}).DoAndReturn(captureEvent(3))

	expectNothingMissing(albums)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.Empty(t, report.Events)
	require.Len(t, report.Notices, 1)
	require.Equal(t, entity.SyncEventFilesRemoved, report.Notices[0].EventType)
	require.Equal(t, int64(3), report.Notices[0].ID)
}

// A folder matched by id under a new name renames its album in place, and the
// rename is recorded as its own activity notice.
func TestSyncImagesRecordsRename(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	// ResolveByFolder returns the album already carrying its new name.
	album := entity.Album{ID: 6, Name: "NewName", FolderID: 600, PositiveRating: 42}

	albums.EXPECT().Count(ctx, countQuery()).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 61, Name: "a.jpg", ParentFolderName: "NewName", ParentFolderID: 600, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(600), "NewName", testDefaultSendMode).
		Return(album, repo.AlbumResolution{RenamedFrom: "OldName"}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{61})

	events.EXPECT().Insert(ctx, entity.SyncEvent{
		EventType:    entity.SyncEventAlbumRenamed,
		AlbumID:      6,
		AlbumName:    "NewName",
		PreviousName: "OldName",
	}).DoAndReturn(captureEvent(4))

	expectNothingMissing(albums)

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	// A rename is not new content: nothing goes to Discord.
	require.Empty(t, report.Events)
	require.Len(t, report.Notices, 1)
	require.Equal(t, entity.SyncEventAlbumRenamed, report.Notices[0].EventType)
	require.Equal(t, "OldName", report.Notices[0].PreviousName)
}

func TestSyncImagesMarksVanishedAlbum(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	album := entity.Album{ID: 1, Name: "Kept"}

	albums.EXPECT().Count(ctx, countQuery()).Return(2, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 11, Name: "a.jpg", ParentFolderName: "Kept", ParentFolderID: 100, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(100), "Kept", testDefaultSendMode).
		Return(album, repo.AlbumResolution{}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(false, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{11})

	// Only "Kept" was seen, so the album whose folder disappeared is flagged
	// (and reported) rather than deleted...
	albums.EXPECT().ClearMissing(ctx, []string{"Kept"}).Return(nil)
	albums.EXPECT().MarkMissingExcept(ctx, []string{"Kept"}).
		Return([]entity.Album{{ID: 9, Name: "Gone"}}, nil)

	// ...and its files are retired with it, which the activity log records.
	images.EXPECT().SoftDeleteByAlbum(ctx, 9, testSourceName).Return([]entity.Image{
		{ID: 81, URL: "x.jpg", Kind: entity.MediaKindImage},
	}, nil)
	events.EXPECT().Insert(ctx, entity.SyncEvent{
		EventType:     entity.SyncEventAlbumMissing,
		AlbumID:       9,
		AlbumName:     "Gone",
		RemovedImages: 1,
		FileNames:     []string{"x.jpg"},
	}).DoAndReturn(captureEvent(5))

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.Equal(t, []string{"Gone"}, report.MissingAlbums)
	require.False(t, report.EmptyScan)
	require.Len(t, report.Notices, 1)
	require.Equal(t, entity.SyncEventAlbumMissing, report.Notices[0].EventType)
	require.Empty(t, report.Events)
}

func TestSyncImagesClearsMissingWhenFolderReturns(t *testing.T) {
	t.Parallel()

	uc, source, albums, images, events := syncUseCase(t)
	ctx := context.Background()

	// The album is still flagged missing in the DB; seeing it again must clear it.
	flagged := time.Now().Add(-24 * time.Hour)
	album := entity.Album{ID: 5, Name: "Back", MissingSince: &flagged}

	albums.EXPECT().Count(ctx, countQuery()).Return(1, nil)
	source.EXPECT().ListMedia(ctx).Return([]repo.MediaEntry{
		{FileID: 51, Name: "b.jpg", ParentFolderName: "Back", ParentFolderID: 500, Kind: entity.MediaKindImage, Size: 10},
	}, nil)
	albums.EXPECT().ResolveByFolder(ctx, int64(500), "Back", testDefaultSendMode).
		Return(album, repo.AlbumResolution{}, nil)
	images.EXPECT().UpsertByFileID(ctx, gomock.Any()).Return(true, nil)
	noCoverCleanup(ctx, albums, images, album, []int64{51})
	events.EXPECT().Insert(ctx, gomock.Any()).DoAndReturn(captureEvent(1))

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

	albums.EXPECT().Count(ctx, countQuery()).Return(3, nil)
	// A source that succeeds but returns nothing almost always means a broken
	// configuration, so nothing may be flagged: no ClearMissing/MarkMissingExcept
	// expectations are registered, and the mock controller fails if they are called.
	source.EXPECT().ListMedia(ctx).Return(nil, nil)
	_, _ = images, events

	report, err := uc.SyncImages(ctx)

	require.NoError(t, err)
	require.True(t, report.EmptyScan)
	require.Empty(t, report.MissingAlbums)
	require.Empty(t, report.Notices)
}
