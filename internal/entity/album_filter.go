package entity

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Album path filter modes. An empty mode means AlbumFilterAll.

const (

	// AlbumFilterAll applies a rule to every album (the default).

	AlbumFilterAll = "all"

	// AlbumFilterInclude applies a rule only to albums under one of Paths.

	AlbumFilterInclude = "include"

	// AlbumFilterExclude applies a rule to everything except those albums.

	AlbumFilterExclude = "exclude"
)

// AlbumPathFilter narrows which albums a delivery rule applies to, addressed by

// where the album's folder lives (Album.SourcePath) rather than by listing

// albums. That is what makes it survive a crawler: a subtree that keeps growing

// new folders stays covered by one rule.

//

// A path matches the folder itself and everything beneath it, compared

// case-insensitively on whole segments — "Crawler" covers "Crawler/Artist" but

// never "CrawlerOld".

type AlbumPathFilter struct {
	Mode string `json:"mode,omitempty"` // all (default) | include | exclude

	Paths []string `json:"paths,omitempty"` // path prefixes, e.g. ["Crawler"]
}

// ParseAlbumPathFilterMode normalizes and validates a mode string.

// An empty string means "all".

func ParseAlbumPathFilterMode(s string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(s))

	switch mode {

	case "", AlbumFilterAll:

		return AlbumFilterAll, nil

	case AlbumFilterInclude, AlbumFilterExclude:

		return mode, nil

	default:

		return "", fmt.Errorf("invalid album filter mode: %q (want all, include, or exclude)", s)

	}
}

// Normalized returns the filter with its mode canonicalized and its paths

// trimmed of blanks and surrounding slashes. A filter that names no path is

// downgraded to "all": an include filter with nothing to include would silence

// a rule completely, which is never what an empty form field means.

func (f AlbumPathFilter) Normalized() AlbumPathFilter {
	mode, err := ParseAlbumPathFilterMode(f.Mode)
	if err != nil {
		mode = AlbumFilterAll
	}

	paths := make([]string, 0, len(f.Paths))

	for _, p := range f.Paths {
		if trimmed := strings.Trim(strings.TrimSpace(p), "/"); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}

	if len(paths) == 0 {
		return AlbumPathFilter{Mode: AlbumFilterAll}
	}

	return AlbumPathFilter{Mode: mode, Paths: paths}
}

// Active reports whether the filter actually narrows anything, so callers can

// skip the work of applying it.

func (f AlbumPathFilter) Active() bool {
	n := f.Normalized()

	return n.Mode != AlbumFilterAll && len(n.Paths) > 0
}

// Matches reports whether an album at sourcePath is in the rule's scope.

//

// An album with no recorded path (a library that has not been synced since the

// path column was added) is treated as "not under any of these paths": an

// exclude filter still covers it, an include filter does not. Erring that way

// keeps an unsynced album in the daily push rather than silently promoting it

// into a subtree it may not belong to.

func (f AlbumPathFilter) Matches(sourcePath string) bool {
	n := f.Normalized()

	if n.Mode == AlbumFilterAll {
		return true
	}

	under := false

	for _, p := range n.Paths {
		if pathIsUnder(sourcePath, p) {

			under = true

			break

		}
	}

	if n.Mode == AlbumFilterInclude {
		return under
	}

	return !under
}

// pathIsUnder reports whether path is prefix itself or sits beneath it.

// Comparison is case-insensitive and segment-wise.

func pathIsUnder(path, prefix string) bool {
	p := strings.ToLower(strings.Trim(strings.TrimSpace(path), "/"))

	q := strings.ToLower(strings.Trim(strings.TrimSpace(prefix), "/"))

	if p == "" || q == "" {
		return false
	}

	return p == q || strings.HasPrefix(p, q+"/")
}

// ParseAlbumPathFilter decodes raw (a rule's stored album_filter) into a filter.

// An empty string or "{}" returns the zero value, which means "every album".

func ParseAlbumPathFilter(raw string) (AlbumPathFilter, error) {
	raw = strings.TrimSpace(raw)

	if raw == "" || raw == "{}" {
		return AlbumPathFilter{}, nil
	}

	var f AlbumPathFilter

	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return AlbumPathFilter{}, fmt.Errorf("invalid album filter: %w", err)
	}

	return f, nil
}

// JSON renders the filter for storage. An inactive filter is stored as "{}" so

// the column stays clean rather than accumulating {"mode":"all"} rows.

func (f AlbumPathFilter) JSON() string {
	if !f.Active() {
		return "{}"
	}

	raw, err := json.Marshal(f.Normalized())
	if err != nil {
		return "{}"
	}

	return string(raw)
}

// Describe renders the filter for a log line or a UI summary.

func (f AlbumPathFilter) Describe() string {
	n := f.Normalized()

	if n.Mode == AlbumFilterAll {
		return "all albums"
	}

	return n.Mode + " " + strings.Join(n.Paths, ", ")
}
