package entity_test

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/stretchr/testify/require"
)

func boolPtr(v bool) *bool { return &v }

func TestMergeMessageStylePerField(t *testing.T) {
	t.Parallel()

	appLayer := entity.MessageStyle{UseEmbed: boolPtr(false), Title: "app title", Body: "app body"}
	ruleLayer := entity.MessageStyle{UseEmbed: boolPtr(true), Title: "rule title"}
	albumLayer := entity.MessageStyle{Body: "album body"}

	// The documented scenario: embeds off by default, the rule turns them on and
	// supplies a shared title, and the album contributes only its own body.
	got := entity.MergeMessageStyle(appLayer, ruleLayer, albumLayer)

	require.True(t, got.EmbedEnabled(), "rule should override the app's embed preference")
	require.Equal(t, "rule title", got.Title, "rule title should survive an album that sets no title")
	require.Equal(t, "album body", got.Body, "album body should win over both lower layers")
}

func TestMergeMessageStyleIgnoresEmptyLayers(t *testing.T) {
	t.Parallel()

	base := entity.MessageStyle{UseEmbed: boolPtr(true), Title: "base", Body: "body"}

	// Empty and whitespace-only fields mean "inherit", not "clear".
	got := entity.MergeMessageStyle(base, entity.MessageStyle{}, entity.MessageStyle{Title: "   "})

	require.True(t, got.EmbedEnabled())
	require.Equal(t, "base", got.Title)
	require.Equal(t, "body", got.Body)
}

func TestMessageStyleEmbedDefault(t *testing.T) {
	t.Parallel()

	// Nothing set anywhere falls back to the built-in preference.
	require.Equal(t, entity.DefaultUseEmbed, entity.MessageStyle{}.EmbedEnabled())
	require.False(t, entity.MessageStyle{UseEmbed: boolPtr(false)}.EmbedEnabled())
}
