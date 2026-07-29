package discord

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

var testAlbum = entity.Album{ID: 7, Name: "Vacation", SendMode: entity.AlbumSendModeRandom, PositiveRating: 42}

func TestRenderCaption(t *testing.T) {
	tokens := albumTokens(testAlbum, 3, albumCounts{Images: 40, Videos: 5, Known: true}, sendContext{})

	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{"empty template falls back", "", "fallback"},
		{"whitespace-only template also falls back", "   ", "fallback"},
		{"album placeholder", "Album: {album}", "Album: Vacation"},
		{"shown counts this message", "{shown} sent", "3 sent"},
		{"album totals count the album", "{album_images}+{album_videos}={album_total}", "40+5=45"},
		{"rating placeholder", "Rated {rating}", "Rated 42"},
		{"unknown placeholder left as-is", "{album} says {unknown}", "Vacation says {unknown}"},
		// The point of 'leave it verbatim': a discovery-only placeholder in a
		// scheduled send must look wrong rather than quietly render "0".
		{"out-of-context placeholder left as-is", "{album}: {new_images}", "Vacation: {new_images}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderCaption(tt.tmpl, tokens, "fallback")
			if got != tt.want {
				t.Fatalf("renderCaption(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
	}
}

func TestAlbumTokensUnknownCounts(t *testing.T) {
	tokens := albumTokens(testAlbum, 3, albumCounts{}, sendContext{})
	const tmpl = "{album}: {album_total}"
	// An unreadable count must not masquerade as an empty album.
	if got := renderCaption(tmpl, tokens, ""); got != "Vacation: {album_total}" {
		t.Fatalf("renderCaption(%q) = %q, want the placeholder left intact", tmpl, got)
	}
}

func TestDiscoveryTokens(t *testing.T) {
	ev := entity.SyncEvent{NewImages: 2, NewVideos: 1}
	album := entity.Album{ID: 3, Name: "Memes"}
	tokens := discoveryTokens(album, ev, 2, albumCounts{Images: 10, Videos: 1, Known: true}, sendContext{})

	const tmpl = "{album}: +{new_images}/{new_videos} = {new_total}, now {album_total}, mode {mode}"
	want := "Memes: +2/1 = 3, now 11, mode {mode}"
	if got := renderCaption(tmpl, tokens, ""); got != want {
		t.Fatalf("renderCaption(%q) = %q, want %q", tmpl, got, want)
	}
}

func TestTestMarker(t *testing.T) {
	tokens := albumTokens(testAlbum, 1, albumCounts{}, sendContext{Test: true})
	plain := false
	style := entity.MessageStyle{UseEmbed: &plain}

	// No title configured: the marker leads the body, not an invented headline.
	msg := renderMessage(style, tokens, "Vacation", true)
	if got := plainContent(msg); got != "[TEST] Vacation" {
		t.Errorf("plainContent = %q, want %q", got, "[TEST] Vacation")
	}

	// With a title, the marker leads the title instead.
	style.Title = "Daily {album}"
	msg = renderMessage(style, tokens, "body", true)
	if got := plainContent(msg); got != "**[TEST] Daily Vacation**\nbody" {
		t.Errorf("plainContent = %q, want the marker on the title", got)
	}

	// Embeds get it on the title too, including the album-name fallback.
	embed := albumEmbed(testAlbum, "", renderMessage(entity.MessageStyle{}, tokens, "body", true))
	if embed.Title != "[TEST] Vacation" {
		t.Errorf("embed.Title = %q, want %q", embed.Title, "[TEST] Vacation")
	}
}

func TestAlbumEmbed(t *testing.T) {
	album := entity.Album{ID: 7, Name: "Vacation", SendMode: entity.AlbumSendModeOrder}
	embed := albumEmbed(album, "https://example.com/thumb.png", renderedMessage{Body: "some description"})

	if embed.Title != "Vacation" {
		t.Errorf("Title = %q, want %q", embed.Title, "Vacation")
	}
	if embed.Description != "some description" {
		t.Errorf("Description = %q, want %q", embed.Description, "some description")
	}
	if embed.Footer == nil || embed.Footer.Text != "album #7 · Order" {
		t.Errorf("Footer = %+v, want text %q", embed.Footer, "album #7 · Order")
	}
	if embed.Thumbnail == nil || embed.Thumbnail.URL != "https://example.com/thumb.png" {
		t.Errorf("Thumbnail = %+v, want URL set", embed.Thumbnail)
	}
	if embed.Color != embedColorByMode[entity.AlbumSendModeOrder] {
		t.Errorf("Color = %#x, want %#x", embed.Color, embedColorByMode[entity.AlbumSendModeOrder])
	}

	noThumb := albumEmbed(entity.Album{ID: 1, Name: "X"}, "", renderedMessage{Body: "desc"})
	if noThumb.Thumbnail != nil {
		t.Errorf("Thumbnail = %+v, want nil when thumbURL is empty", noThumb.Thumbnail)
	}
	if noThumb.Footer.Text != "album #1" {
		t.Errorf("Footer = %q, want %q (no mode suffix)", noThumb.Footer.Text, "album #1")
	}
}
