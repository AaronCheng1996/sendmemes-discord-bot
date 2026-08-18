package discord

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

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

	notice := oversizedNotice([]fileEntry{fe("huge.png", 30*1024*1024), fe("big.gif", 2048)}, 20*1024*1024)
	for _, want := range []string{"2 file(s)", "20.0 MB", "huge.png", "30.0 MB", "big.gif", "2.0 KB"} {
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
		{20 * 1024 * 1024, "20.0 MB"},
	} {
		if got := humanBytes(tt.in); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func limitBot(mb int) *Bot {
	cfg := &config.Config{}
	cfg.Discord.UploadLimitMB = mb
	return &Bot{cfg: cfg, l: noopLogger{}}
}

func TestUploadLimit(t *testing.T) {
	t.Parallel()

	// No session, so the boost lookup always misses and the configured floor
	// stands — the path every DM and uncached thread takes.
	tests := []struct {
		name string
		mb   int
		want int
	}{
		{name: "free tier", mb: 20, want: 20*1024*1024 - uploadOverheadBytes},
		{name: "nitro basic", mb: 50, want: 50*1024*1024 - uploadOverheadBytes},
		{name: "the old 10 MB cap", mb: 10, want: 10*1024*1024 - uploadOverheadBytes},
		// Misconfiguration must not yield a budget nothing fits in.
		{name: "zero falls back", mb: 0, want: minUploadBudget},
		{name: "negative falls back", mb: -5, want: minUploadBudget},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := limitBot(tt.mb).uploadLimit("chan"); got != tt.want {
				t.Fatalf("uploadLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

// The budget must stay strictly under what Discord advertises: the API rejects
// an over-budget request whole, losing every attachment in it.
func TestUploadLimitLeavesHeadroom(t *testing.T) {
	t.Parallel()

	for _, mb := range []int{10, 20, 50, 100} {
		if got := limitBot(mb).uploadLimit("chan"); got >= mb*1024*1024 {
			t.Errorf("uploadLimit() = %d for a %d MB cap, want headroom below it", got, mb)
		}
	}
}

func TestGuildBoostLimitWithoutSession(t *testing.T) {
	t.Parallel()

	if _, ok := limitBot(20).guildBoostLimit("chan"); ok {
		t.Error("guildBoostLimit reported a limit with no session state")
	}
}

// restErr builds the error shape discordgo returns for a non-2xx API response.
func restErr(code int) error {
	return &discordgo.RESTError{
		Response: &http.Response{StatusCode: http.StatusRequestEntityTooLarge},
		Message:  &discordgo.APIErrorMessage{Code: code, Message: "Request entity too large"},
	}
}

func TestRestErrorCode(t *testing.T) {
	t.Parallel()

	if got := restErrorCode(restErr(discordgo.ErrCodeRequestEntityTooLarge)); got != 40005 {
		t.Errorf("restErrorCode(40005 REST error) = %d, want 40005", got)
	}
	// Wrapped, because the send helpers pass errors up through fmt.Errorf.
	wrapped := errors.Join(errors.New("context"), restErr(discordgo.ErrCodeRequestEntityTooLarge))
	if got := restErrorCode(wrapped); got != 40005 {
		t.Errorf("restErrorCode(wrapped) = %d, want 40005", got)
	}
	if got := restErrorCode(errors.New("connection reset")); got != 0 {
		t.Errorf("restErrorCode(plain error) = %d, want 0", got)
	}
	if got := restErrorCode(nil); got != 0 {
		t.Errorf("restErrorCode(nil) = %d, want 0", got)
	}
}

func TestSendFailureNotice(t *testing.T) {
	t.Parallel()

	files := []*discordgo.File{{Name: "a.gif"}, {Name: "b.gif"}}

	// 40005 is the one an operator can act on, so it names the setting, and it
	// quotes the budget the bot actually applied rather than the raw setting.
	budget := limitBot(20).uploadLimit("chan")
	tooLarge := sendFailureNotice(files, restErr(discordgo.ErrCodeRequestEntityTooLarge), budget)
	for _, want := range []string{"2 attachment(s)", "19.5 MB", "DISCORD_UPLOAD_LIMIT_MB"} {
		if !strings.Contains(tooLarge, want) {
			t.Errorf("40005 notice missing %q:\n%s", want, tooLarge)
		}
	}

	other := sendFailureNotice(files, errors.New("connection reset"), budget)
	if strings.Contains(other, "DISCORD_UPLOAD_LIMIT_MB") {
		t.Errorf("generic notice should not blame the size setting:\n%s", other)
	}
	if other == "" {
		t.Error("generic failure produced no notice")
	}
}

// A full-album post is dozens of messages; whatever breaks one usually breaks
// the rest, and forty identical notices is a second outage.
func TestFailureNoticeIsThrottledPerChannel(t *testing.T) {
	t.Parallel()

	b := limitBot(20)
	b.lastNotice = make(map[string]time.Time)

	if !b.claimFailureNotice("chan-a") {
		t.Fatal("first notice for a channel was suppressed")
	}
	if b.claimFailureNotice("chan-a") {
		t.Error("second notice within the interval was not suppressed")
	}
	// Throttling is per channel: a different channel still deserves its own.
	if !b.claimFailureNotice("chan-b") {
		t.Error("another channel was suppressed by the first channel's notice")
	}

	b.lastNotice["chan-a"] = time.Now().Add(-sendFailureNoticeInterval - time.Second)
	if !b.claimFailureNotice("chan-a") {
		t.Error("notice still suppressed after the interval elapsed")
	}
}
