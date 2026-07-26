// Package localfs implements a MediaSource backed by a directory tree on the
// local filesystem, so a self-hosted instance can run without a pCloud
// account: point MEDIA_LOCAL_ROOT at a mounted folder of "album/file" trees.
package localfs

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// Source implements repo.MediaSource over files rooted at Root. Files must
// sit inside an album subfolder (Root/<album>/<file>, arbitrarily nested);
// files directly under Root are skipped, matching the pCloud source's
// semantics of "immediate parent folder = album name".
type Source struct {
	root       string
	publicBase string // HTTP_PUBLIC_URL, base for the exposed /media/* route
}

// New creates a local filesystem MediaSource. root is the directory walked
// by ListMedia and served by the /media/* route; publicBase is the
// externally reachable base URL that route is mounted under (HTTP_PUBLIC_URL).
func New(root, publicBase string) *Source {
	return &Source{
		root:       filepath.Clean(root),
		publicBase: strings.TrimSuffix(publicBase, "/"),
	}
}

// Root returns the configured root directory (for wiring the /media/* route).
func (s *Source) Root() string {
	return s.root
}

// ListMedia walks Root and returns every recognized media file found inside
// an album subfolder. Files directly under Root, and files with an
// unrecognized extension, are skipped.
func (s *Source) ListMedia(_ context.Context) ([]repo.MediaEntry, error) {
	var entries []repo.MediaEntry
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return fmt.Errorf("localfs - ListMedia - Rel %q: %w", path, err)
		}
		dir := filepath.Dir(rel)
		if dir == "." {
			return nil // root-level file: no album folder, skip (matches pCloud semantics)
		}
		kind, ok := entity.KindOfExtension(d.Name())
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("localfs - ListMedia - Info %q: %w", path, err)
		}
		entries = append(entries, repo.MediaEntry{
			FileID:           fileID(filepath.ToSlash(rel)),
			Name:             d.Name(),
			ParentFolderName: filepath.Base(dir),
			Kind:             kind,
			Size:             info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("localfs - ListMedia - WalkDir: %w", err)
	}
	return entries, nil
}

// fileID derives a stable, positive file identifier from a file's path
// relative to Root, using FNV-64a. Collisions are theoretically possible but
// vanishingly unlikely for any realistic media library; images.file_id has a
// unique index, so a collision would surface as an upsert conflict rather
// than silently merging two different files.
func fileID(relPath string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(relPath))              // hash.Hash.Write never returns an error
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF) //nolint:gosec // masked to the positive range, always fits int64
}

// ResolveDownloadURL returns the static /media/* URL for m. Local files have
// no expiry or IP-binding concerns, so this is identical to ResolveShareURL.
func (s *Source) ResolveDownloadURL(_ context.Context, m entity.Image) (string, error) {
	return s.mediaURL(m), nil
}

// ResolveShareURL returns the static /media/* URL for m. It never expires,
// same as ResolveDownloadURL, since local files have nothing to distinguish
// "temporary" from "permanent" access.
func (s *Source) ResolveShareURL(_ context.Context, m entity.Image) (string, error) {
	return s.mediaURL(m), nil
}

// ThumbURL has no thumbnail service to call, so the original file URL is
// returned directly — the browser or Discord renders the full image as its
// own thumbnail.
func (s *Source) ThumbURL(shareURL string, _ entity.Image, _ string) string {
	return shareURL
}

// mediaURL reconstructs m's path relative to Root from its album name and
// filename (the same two fields ListMedia derived them from) and builds the
// externally reachable URL served by the /media/* route.
func (s *Source) mediaURL(m entity.Image) string {
	segments := strings.Split(filepath.ToSlash(filepath.Join(m.AlbumName, m.URL)), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return s.publicBase + "/media/" + strings.Join(segments, "/")
}
