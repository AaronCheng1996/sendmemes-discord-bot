package usecase_test

import (
	"testing"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	settingsuc "github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase/settings"
	"github.com/stretchr/testify/require"
)

func settingsTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Discord.SendChannelID = "chan-env"
	cfg.Discord.SendInterval = "6h"
	cfg.Discord.SendHistorySize = 5
	cfg.Discord.NotifyChannelID = "notify-env"
	return cfg
}

func TestEffectiveScheduleNotifyChannelFallback(t *testing.T) {
	t.Parallel()

	cfg := settingsTestConfig()

	// No DB row: every field falls back to env, including the notify channel.
	out := settingsuc.EffectiveSchedule(cfg, entity.DiscordScheduleSettings{})
	require.Equal(t, "notify-env", out.NotifyChannelID)
	require.Equal(t, "env", out.SourceNotifyChannelID)
	require.Equal(t, "chan-env", out.SendChannelID)

	// DB override wins and is attributed to "db".
	out = settingsuc.EffectiveSchedule(cfg, entity.DiscordScheduleSettings{
		GuildID:         "g1",
		NotifyChannelID: "notify-db",
	})
	require.Equal(t, "notify-db", out.NotifyChannelID)
	require.Equal(t, "db", out.SourceNotifyChannelID)
	// Fields absent from the DB row still come from env.
	require.Equal(t, "chan-env", out.SendChannelID)
	require.Equal(t, "env", out.SourceSendChannelID)
}
