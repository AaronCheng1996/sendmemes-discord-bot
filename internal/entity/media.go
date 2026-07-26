package entity

import (
	"path/filepath"
	"strings"
)

// Media source labels recorded on Image.Source, identifying which configured
// MediaSource resolved and owns the image's file. ImagesUseCase resolves URLs
// through a MediaSource only for these two values; images with any other
// Source (e.g. manually created via the admin API with a literal URL) are
// treated as already-resolved plain URLs.
const (
	MediaSourcePCloud = "pcloud"
	MediaSourceLocal  = "local"
)

// MediaExtensions maps a lowercased file extension to its media kind
// (MediaKindImage or MediaKindVideo). Extensions absent from the map are not
// recognized as media by any MediaSource implementation.
var MediaExtensions = map[string]string{
	".jpg":  MediaKindImage,
	".jpeg": MediaKindImage,
	".png":  MediaKindImage,
	".gif":  MediaKindImage,
	".webp": MediaKindImage,
	".mp4":  MediaKindVideo,
	".webm": MediaKindVideo,
	".mov":  MediaKindVideo,
	".m4v":  MediaKindVideo,
	".mkv":  MediaKindVideo,
	".avi":  MediaKindVideo,
}

// KindOfExtension returns the media kind for name's file extension and
// whether the extension is recognized as media.
func KindOfExtension(name string) (string, bool) {
	kind, ok := MediaExtensions[strings.ToLower(filepath.Ext(name))]
	return kind, ok
}

// IsManagedMediaSource reports whether source is a MediaSource-backed image
// (as opposed to a manually created one with a literal/absolute URL).
func IsManagedMediaSource(source string) bool {
	return source == MediaSourcePCloud || source == MediaSourceLocal
}
