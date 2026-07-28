package entity_test

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/stretchr/testify/require"
)

func TestParseAlbumSendMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    entity.AlbumSendMode
		wantErr bool
	}{
		{name: "empty defaults to random", input: "", want: entity.AlbumSendModeRandom},
		{name: "whitespace defaults to random", input: "   ", want: entity.AlbumSendModeRandom},
		{name: "order", input: "Order", want: entity.AlbumSendModeOrder},
		{name: "random", input: "Random", want: entity.AlbumSendModeRandom},
		{name: "single", input: "Single", want: entity.AlbumSendModeSingle},
		{name: "video", input: "Video", want: entity.AlbumSendModeVideo},
		{name: "custom", input: "Custom", want: entity.AlbumSendModeCustom},
		{name: "trims surrounding whitespace", input: "  Video  ", want: entity.AlbumSendModeVideo},
		{name: "invalid returns error", input: "Bogus", wantErr: true},
		{name: "wrong case is invalid", input: "video", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := entity.ParseAlbumSendMode(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, string(got))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseAlbumSendConfig(t *testing.T) {
	t.Parallel()

	falseVal := false
	trueVal := true

	tests := []struct {
		name    string
		input   string
		want    entity.AlbumSendConfig
		wantErr bool
	}{
		{name: "empty string is zero value", input: "", want: entity.AlbumSendConfig{}},
		{name: "empty object is zero value", input: "{}", want: entity.AlbumSendConfig{}},
		{
			name:  "all fields",
			input: `{"batch_size":5,"include_cover":false,"ordered":true,"caption":"hi {album}","nsfw":true}`,
			want: entity.AlbumSendConfig{
				BatchSize:    5,
				IncludeCover: &falseVal,
				Ordered:      true,
				Caption:      "hi {album}",
				NSFW:         true,
			},
		},
		{
			name:  "include_cover true is distinguishable from unset",
			input: `{"include_cover":true}`,
			want:  entity.AlbumSendConfig{IncludeCover: &trueVal},
		},
		{
			name:  "partial fields keep the rest zero",
			input: `{"batch_size":3}`,
			want:  entity.AlbumSendConfig{BatchSize: 3},
		},
		{name: "malformed JSON is an error", input: `{"batch_size":`, wantErr: true},
		{name: "wrong type is an error", input: `{"batch_size":"five"}`, wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := entity.ParseAlbumSendConfig(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				require.Equal(t, entity.AlbumSendConfig{}, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
