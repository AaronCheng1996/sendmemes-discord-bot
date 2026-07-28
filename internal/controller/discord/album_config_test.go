package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

func TestEffectiveCaptionTemplate(t *testing.T) {
	tests := []struct {
		name     string
		caption  string
		fallback string
		want     string
	}{
		{name: "empty caption keeps fallback", caption: "", fallback: "{album}", want: "{album}"},
		{name: "whitespace-only caption keeps fallback", caption: "   ", fallback: "{album}", want: "{album}"},
		{name: "caption overrides fallback", caption: "custom {album}", fallback: "{album}", want: "custom {album}"},
		{name: "caption overrides empty fallback", caption: "custom", fallback: "", want: "custom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveCaptionTemplate(entity.AlbumSendConfig{Caption: tt.caption}, tt.fallback)
			if got != tt.want {
				t.Errorf("effectiveCaptionTemplate(%q, %q) = %q, want %q", tt.caption, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestBatchSizeOrDefault(t *testing.T) {
	tests := []struct {
		name      string
		batchSize int
		fall      int
		want      int
	}{
		{name: "zero uses fallback", batchSize: 0, fall: 10, want: 10},
		{name: "negative uses fallback", batchSize: -1, fall: 10, want: 10},
		{name: "positive overrides fallback", batchSize: 3, fall: 10, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := batchSizeOrDefault(entity.AlbumSendConfig{BatchSize: tt.batchSize}, tt.fall)
			if got != tt.want {
				t.Errorf("batchSizeOrDefault(%d, %d) = %d, want %d", tt.batchSize, tt.fall, got, tt.want)
			}
		})
	}
}

func TestApplySpoiler(t *testing.T) {
	t.Run("nsfw false leaves names untouched", func(t *testing.T) {
		files := []*discordgo.File{{Name: "a.png"}, {Name: "b.png"}}
		applySpoiler(files, false)
		if files[0].Name != "a.png" || files[1].Name != "b.png" {
			t.Errorf("names changed unexpectedly: %+v", files)
		}
	})

	t.Run("nsfw true prefixes every file", func(t *testing.T) {
		files := []*discordgo.File{{Name: "a.png"}, {Name: "b.png"}}
		applySpoiler(files, true)
		if files[0].Name != "SPOILER_a.png" || files[1].Name != "SPOILER_b.png" {
			t.Errorf("names not prefixed: %+v", files)
		}
	})

	t.Run("nsfw true does not double-prefix an already-spoilered name", func(t *testing.T) {
		files := []*discordgo.File{{Name: "SPOILER_a.png"}}
		applySpoiler(files, true)
		if files[0].Name != "SPOILER_a.png" {
			t.Errorf("name double-prefixed: %q", files[0].Name)
		}
	})
}

func TestExcludeCover(t *testing.T) {
	imgs := []entity.Image{
		{ID: 1, IsCover: true},
		{ID: 2},
		{ID: 3},
	}
	got := excludeCover(imgs)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Errorf("excludeCover = %+v, want ids [2 3]", got)
	}

	// The input slice itself is untouched.
	if !imgs[0].IsCover {
		t.Errorf("excludeCover mutated its input")
	}

	none := excludeCover([]entity.Image{{ID: 5}})
	if len(none) != 1 {
		t.Errorf("excludeCover with no cover = %+v, want unchanged", none)
	}
}
