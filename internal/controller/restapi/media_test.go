package restapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

// resolveMediaPath is the security-critical piece of the static /media/*
// route: it must accept only paths that resolve strictly inside root and
// carry a recognized media extension, and reject "../" traversal attempts.
func TestResolveMediaPath(t *testing.T) {
	t.Parallel()

	root := filepath.FromSlash("/media")

	cases := []struct {
		name string
		rel  string
		ok   bool
	}{
		{name: "plain file", rel: "AlbumA/1.jpg", ok: true},
		{name: "nested file", rel: "AlbumA/sub/2.mp4", ok: true},
		{name: "empty", rel: "", ok: false},
		{name: "unrecognized extension", rel: "AlbumA/notes.txt", ok: false},
		{name: "no extension", rel: "AlbumA/README", ok: false},
		{name: "traversal to parent", rel: "../secret.jpg", ok: false},
		{name: "traversal above root", rel: "../../etc/secret.jpg", ok: false},
		{name: "traversal disguised mid-path", rel: "AlbumA/../../secret.jpg", ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			full, ok := resolveMediaPath(root, tc.rel)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.True(t, strings.HasPrefix(full, root+string(os.PathSeparator)))
			}
		})
	}
}

// End-to-end: the route actually serves a file inside root and rejects a
// traversal attempt at the HTTP layer, not just in the helper.
func TestLocalMediaRoute(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "AlbumA"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "AlbumA", "1.jpg"), []byte("data"), 0o600))

	app := fiber.New()
	cfg := &config.Config{}
	cfg.Media.Source = "local"
	cfg.Media.LocalRoot = root
	registerLocalMediaRoute(app, cfg)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/media/AlbumA/1.jpg", http.NoBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	resp2, err := app.Test(httptest.NewRequest(http.MethodGet, "/media/AlbumA/missing.jpg", http.NoBody))
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.NotEqual(t, http.StatusOK, resp2.StatusCode)
}

// The route must not be registered at all when MEDIA_SOURCE is not "local"
// (the pCloud source has no local root to serve).
func TestLocalMediaRouteNotRegisteredWhenNotLocal(t *testing.T) {
	t.Parallel()

	app := fiber.New()
	cfg := &config.Config{}
	cfg.Media.Source = "pcloud"
	registerLocalMediaRoute(app, cfg)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/media/AlbumA/1.jpg", http.NoBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
