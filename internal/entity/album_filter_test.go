package entity_test

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/stretchr/testify/require"
)

func TestAlbumPathFilterMatches(t *testing.T) {
	t.Parallel()

	exclude := entity.AlbumPathFilter{Mode: entity.AlbumFilterExclude, Paths: []string{"Media/Crawler"}}

	include := entity.AlbumPathFilter{Mode: entity.AlbumFilterInclude, Paths: []string{"Media/Crawler"}}

	tests := []struct {
		name string

		path string

		wantIn, wantOut bool // wantIn = include filter matches, wantOut = exclude filter matches
	}{
		{name: "the folder itself", path: "Media/Crawler", wantIn: true, wantOut: false},

		{name: "an album under it", path: "Media/Crawler/SomeArtist", wantIn: true, wantOut: false},

		{name: "nested deeper", path: "Media/Crawler/SomeArtist/2026", wantIn: true, wantOut: false},

		{name: "case differs", path: "media/crawler/someartist", wantIn: true, wantOut: false},

		{name: "trailing slash", path: "Media/Crawler/SomeArtist/", wantIn: true, wantOut: false},

		// The prefix has to end on a segment boundary, or "Crawler" would

		// swallow every sibling whose name merely starts the same way.

		{name: "sibling with a shared prefix", path: "Media/CrawlerOld", wantIn: false, wantOut: true},

		{name: "unrelated album", path: "Media/Memes", wantIn: false, wantOut: true},

		// An album synced before source_path existed has no path. Excluding it

		// would silently drop it out of the daily push, so it stays in.

		{name: "no recorded path", path: "", wantIn: false, wantOut: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.wantIn, include.Matches(tt.path), "include")

			require.Equal(t, tt.wantOut, exclude.Matches(tt.path), "exclude")
		})
	}
}

func TestAlbumPathFilterAllMatchesEverything(t *testing.T) {
	t.Parallel()

	for _, f := range []entity.AlbumPathFilter{
		{},

		{Mode: entity.AlbumFilterAll},

		// A mode with nothing to match on is a half-filled form, not a rule that

		// covers nothing — an include filter here would silence the rule.

		{Mode: entity.AlbumFilterInclude},

		{Mode: entity.AlbumFilterExclude, Paths: []string{"  ", "/"}},
	} {

		require.False(t, f.Active())

		require.True(t, f.Matches("Media/Anything"))

		require.True(t, f.Matches(""))

	}
}

func TestParseAlbumPathFilterMode(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"", "  ", "all", "ALL"} {

		mode, err := entity.ParseAlbumPathFilterMode(in)

		require.NoError(t, err)

		require.Equal(t, entity.AlbumFilterAll, mode)

	}

	mode, err := entity.ParseAlbumPathFilterMode(" Include ")

	require.NoError(t, err)

	require.Equal(t, entity.AlbumFilterInclude, mode)

	_, err = entity.ParseAlbumPathFilterMode("only")

	require.Error(t, err)
}

func TestAlbumPathFilterRoundTrip(t *testing.T) {
	t.Parallel()

	f := entity.AlbumPathFilter{Mode: entity.AlbumFilterExclude, Paths: []string{" /Crawler/ ", "", "Drafts"}}

	back, err := entity.ParseAlbumPathFilter(f.JSON())

	require.NoError(t, err)

	require.Equal(t, entity.AlbumFilterExclude, back.Mode)

	require.Equal(t, []string{"Crawler", "Drafts"}, back.Paths)

	// An inactive filter is stored as an empty object rather than {"mode":"all"}.

	require.Equal(t, "{}", entity.AlbumPathFilter{Mode: entity.AlbumFilterAll}.JSON())

	empty, err := entity.ParseAlbumPathFilter("")

	require.NoError(t, err)

	require.True(t, empty.Matches("Media/Anything"))

	_, err = entity.ParseAlbumPathFilter("not json")

	require.Error(t, err)
}
