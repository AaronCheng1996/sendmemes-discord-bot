package localfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
}

// ListMedia must skip root-level files and unrecognized extensions, and
// derive each entry's album name from its immediate parent folder, however
// deeply nested.
func TestListMedia(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "AlbumA", "1.jpg"))
	writeFile(t, filepath.Join(root, "AlbumA", "sub", "2.mp4"))
	writeFile(t, filepath.Join(root, "AlbumA", "notes.txt")) // unrecognized extension
	writeFile(t, filepath.Join(root, "root-level.jpg"))      // no album folder, skipped

	src := New(root, "https://example.test")
	entries, err := src.ListMedia(context.Background())
	require.NoError(t, err)
	require.Len(t, entries, 2)

	byName := make(map[string]repo.MediaEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	img, ok := byName["1.jpg"]
	require.True(t, ok)
	require.Equal(t, "AlbumA", img.ParentFolderName)
	require.Equal(t, entity.MediaKindImage, img.Kind)
	require.EqualValues(t, len("data"), img.Size)

	video, ok := byName["2.mp4"]
	require.True(t, ok)
	require.Equal(t, "sub", video.ParentFolderName) // immediate parent, not "AlbumA"
	require.Equal(t, entity.MediaKindVideo, video.Kind)
}

// FileID must be stable across calls for the same relative path (so re-syncs
// upsert instead of duplicating), and distinct paths must not collide in
// this test's fixed inputs.
func TestFileIDStable(t *testing.T) {
	t.Parallel()

	require.Equal(t, fileID("AlbumA/1.jpg"), fileID("AlbumA/1.jpg"))
	require.NotEqual(t, fileID("AlbumA/1.jpg"), fileID("AlbumA/2.jpg"))
	require.GreaterOrEqual(t, fileID("AlbumA/1.jpg"), int64(0))
}

// ResolveDownloadURL and ResolveShareURL both reconstruct the file's path
// under Root from AlbumName + URL (the two fields ListMedia derived them
// from) and must agree, since local files have no expiry/IP concerns.
func TestResolveURLs(t *testing.T) {
	t.Parallel()

	src := New("/media", "https://example.test/")
	img := entity.Image{AlbumName: "Album A", URL: "pic 1.jpg"}

	want := "https://example.test/media/Album%20A/pic%201.jpg"

	got, err := src.ResolveDownloadURL(context.Background(), img)
	require.NoError(t, err)
	require.Equal(t, want, got)

	got, err = src.ResolveShareURL(context.Background(), img)
	require.NoError(t, err)
	require.Equal(t, want, got)

	require.Equal(t, want, src.ThumbURL(want, img, ""))
}
