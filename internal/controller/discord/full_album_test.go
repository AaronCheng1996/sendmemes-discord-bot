package discord

import (
	"strings"
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

func pagingBot(threshold, pageSize int) *Bot {
	cfg := &config.Config{}
	cfg.Discord.FullAlbumPageThreshold = threshold
	cfg.Discord.FullAlbumPageSize = pageSize
	return &Bot{cfg: cfg, l: noopLogger{}}
}

func TestFullAlbumPaging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		threshold    int
		pageSize     int
		total        int
		wantPageSize int
		wantPaged    bool
	}{
		{name: "below threshold posts everything", threshold: 200, pageSize: 100, total: 150, wantPageSize: 150},
		{name: "at threshold posts everything", threshold: 200, pageSize: 100, total: 200, wantPageSize: 200},
		{name: "above threshold pages", threshold: 200, pageSize: 100, total: 1000, wantPageSize: 100, wantPaged: true},
		{name: "zero threshold disables paging", threshold: 0, pageSize: 100, total: 5000, wantPageSize: 5000},
		{name: "negative threshold disables paging", threshold: -1, pageSize: 100, total: 5000, wantPageSize: 5000},
		// A page size of 0 would advance the offset by nothing, so the continue
		// button would never finish the album.
		{name: "invalid page size falls back", threshold: 200, pageSize: 0, total: 1000,
			wantPageSize: fullAlbumPageSizeFallback, wantPaged: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, paged := pagingBot(tt.threshold, tt.pageSize).fullAlbumPaging(tt.total)
			if got != tt.wantPageSize || paged != tt.wantPaged {
				t.Fatalf("fullAlbumPaging(%d) = (%d, %v), want (%d, %v)",
					tt.total, got, paged, tt.wantPageSize, tt.wantPaged)
			}
		})
	}
}

func TestAlbumRefFrom(t *testing.T) {
	t.Parallel()

	cover := entity.Image{ID: 1, AlbumID: 9, IsCover: true}
	imgs := []entity.Image{{ID: 2, AlbumID: 9}, {ID: 3, AlbumID: 9}}

	if got := albumRefFrom("Trip", cover, true, imgs); got.ID != 9 || got.Name != "Trip" {
		t.Errorf("albumRefFrom with images = %+v, want id 9 name Trip", got)
	}
	// Cover-only albums still need an id for the continue button.
	if got := albumRefFrom("Trip", cover, true, nil); got.ID != 9 {
		t.Errorf("albumRefFrom cover-only = %+v, want id 9", got)
	}
	if got := albumRefFrom("Trip", entity.Image{}, false, nil); got.ID != 0 || got.Name != "Trip" {
		t.Errorf("albumRefFrom empty = %+v, want id 0 name Trip", got)
	}
}

func TestOversizedNotice(t *testing.T) {
	t.Parallel()

	notice := oversizedNotice([]fileEntry{fe("huge.png", 30*1024*1024), fe("big.gif", 2048)})
	for _, want := range []string{"2 file(s)", "huge.png", "30.0 MB", "big.gif", "2.0 KB"} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice missing %q:\n%s", want, notice)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   int
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1536, "1.5 KB"},
		{discordMsgLimit, "24.0 MB"},
	} {
		if got := humanBytes(tt.in); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
