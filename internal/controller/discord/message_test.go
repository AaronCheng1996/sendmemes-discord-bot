package discord

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

func TestRenderCaption(t *testing.T) {
	album := entity.Album{ID: 7, Name: "Vacation", SendMode: entity.AlbumSendModeRandom, PositiveRating: 42}

	tests := []struct {
		name   string
		tmpl   string
		sent   int
		total  int
		prefix string
		want   string
	}{
		{
			name: "empty template falls back to default, no truncation",
			tmpl: "", sent: 5, total: 5, prefix: "",
			want: "Vacation",
		},
		{
			name: "empty template falls back to default, with prefix",
			tmpl: "", sent: 1, total: 1, prefix: "[TEST] ",
			want: "[TEST] Vacation",
		},
		{
			name: "empty template falls back to default, truncated",
			tmpl: "", sent: 3, total: 10, prefix: "",
			want: "Vacation (showing 3 of 10)",
		},
		{
			name: "whitespace-only template also falls back to default",
			tmpl: "   ", sent: 5, total: 5, prefix: "",
			want: "Vacation",
		},
		{
			name: "album placeholder",
			tmpl: "Album: {album}", sent: 1, total: 1, prefix: "",
			want: "Album: Vacation",
		},
		{
			name: "count and total placeholders",
			tmpl: "{count}/{total} images", sent: 3, total: 10, prefix: "",
			want: "3/10 images",
		},
		{
			name: "rating placeholder",
			tmpl: "Rated {rating}", sent: 1, total: 1, prefix: "",
			want: "Rated 42",
		},
		{
			name: "prefix placeholder",
			tmpl: "{prefix}{album}", sent: 1, total: 1, prefix: "[TEST] ",
			want: "[TEST] Vacation",
		},
		{
			name: "all placeholders combined",
			tmpl: "{prefix}{album}: {count}/{total} (rating {rating})", sent: 2, total: 4, prefix: ">> ",
			want: ">> Vacation: 2/4 (rating 42)",
		},
		{
			name: "unknown placeholder left as-is",
			tmpl: "{album} says {unknown}", sent: 1, total: 1, prefix: "",
			want: "Vacation says {unknown}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderCaption(tt.tmpl, album, tt.sent, tt.total, tt.prefix)
			if got != tt.want {
				t.Fatalf("renderCaption(%q) = %q, want %q", tt.tmpl, got, tt.want)
			}
		})
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
