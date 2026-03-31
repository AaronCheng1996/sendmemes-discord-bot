package discord

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

func (b *Bot) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	content := strings.TrimSpace(m.Content)
	if content == "" || content[0] != '!' {
		return
	}
	cmd, arg, _ := strings.Cut(content[1:], " ")
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	arg = strings.TrimSpace(arg)

	ctx := context.Background()
	switch cmd {
	case "ping":
		_, _ = s.ChannelMessageSend(m.ChannelID, "pong")
	case "image":
		go b.msgImage(ctx, s, m.ChannelID)
	case "rng_image":
		go b.msgRngImage(ctx, s, m.ChannelID)
	case "rng_album":
		go b.msgRngAlbum(ctx, s, m.ChannelID)
	case "album":
		if arg == "" {
			_, _ = s.ChannelMessageSend(m.ChannelID, "Usage: `!album <name>`")
			return
		}
		go b.msgAlbum(ctx, s, m.ChannelID, arg)
	case "full_album":
		if arg == "" {
			_, _ = s.ChannelMessageSend(m.ChannelID, "Usage: `!full_album <name>`")
			return
		}
		go b.msgFullAlbum(ctx, s, m.ChannelID, arg)
	case "schedule":
		go b.msgSchedule(ctx, s, m, arg)
	}
}

func (b *Bot) msgSchedule(ctx context.Context, s *discordgo.Session, m *discordgo.MessageCreate, arg string) {
	args := strings.Fields(arg)
	if len(args) == 0 || strings.EqualFold(args[0], "show") {
		effective, err := b.settingsUC.GetEffectiveSchedule(ctx, m.GuildID)
		if err != nil {
			b.l.Error(fmt.Errorf("msgSchedule show: %w", err))
			_, _ = s.ChannelMessageSend(m.ChannelID, "Failed to load schedule settings.")
			return
		}
		_, _ = s.ChannelMessageSend(m.ChannelID, b.scheduleDisplay(effective))
		return
	}

	if !b.hasMessageSchedulePermission(s, m) {
		_, _ = s.ChannelMessageSend(m.ChannelID, "You need Manage Channels permission to update schedule.")
		return
	}
	if !strings.EqualFold(args[0], "set") || len(args) < 4 {
		_, _ = s.ChannelMessageSend(m.ChannelID, "Usage: `!schedule set <channel_id|#channel> <interval> <history_size>`")
		return
	}

	channelID := normalizeChannelArg(args[1])
	interval := strings.TrimSpace(args[2])
	historySize, err := strconv.Atoi(args[3])
	if err != nil || historySize <= 0 {
		_, _ = s.ChannelMessageSend(m.ChannelID, "history_size must be a positive integer.")
		return
	}
	if _, err = timeParseDuration(interval); err != nil {
		_, _ = s.ChannelMessageSend(m.ChannelID, "Invalid interval. Example: 6h, 30m, 24h.")
		return
	}

	if _, err = b.settingsUC.UpsertSchedule(ctx, entity.DiscordScheduleSettings{
		GuildID:         m.GuildID,
		SendChannelID:   channelID,
		SendInterval:    interval,
		SendHistorySize: historySize,
	}); err != nil {
		b.l.Error(fmt.Errorf("msgSchedule set: %w", err))
		_, _ = s.ChannelMessageSend(m.ChannelID, "Failed to update schedule settings.")
		return
	}

	effective, err := b.settingsUC.GetEffectiveSchedule(ctx, m.GuildID)
	if err != nil {
		b.l.Error(fmt.Errorf("msgSchedule set effective: %w", err))
		_, _ = s.ChannelMessageSend(m.ChannelID, "Updated, but failed to reload effective settings.")
		return
	}
	_, _ = s.ChannelMessageSend(m.ChannelID, "Schedule updated.\n"+b.scheduleDisplay(effective))
}

func (b *Bot) hasMessageSchedulePermission(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	perms, err := s.State.UserChannelPermissions(m.Author.ID, m.ChannelID)
	if err != nil {
		perms, err = s.UserChannelPermissions(m.Author.ID, m.ChannelID)
		if err != nil {
			return false
		}
	}
	return perms&discordgo.PermissionManageChannels != 0 || perms&discordgo.PermissionAdministrator != 0
}

func normalizeChannelArg(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "<#") && strings.HasSuffix(v, ">") && len(v) > 3 {
		return strings.TrimSuffix(strings.TrimPrefix(v, "<#"), ">")
	}
	return v
}

func (b *Bot) msgImage(ctx context.Context, s *discordgo.Session, channelID string) {
	b.vlog("!image received in channel %s", channelID)
	img, err := b.imagesUC.GetImage(ctx)
	if err != nil {
		b.l.Error(fmt.Errorf("msgImage: %w", err))
		_, _ = s.ChannelMessageSend(channelID, "Failed to get image.")
		return
	}
	files, err := b.downloadImages(ctx, []entity.Image{img})
	if err != nil {
		b.l.Error(fmt.Errorf("msgImage download: %w", err))
		return
	}
	b.channelSendFiles(s, channelID, "", files)
	b.vlog("!image completed in channel %s", channelID)
}

func (b *Bot) msgRngImage(ctx context.Context, s *discordgo.Session, channelID string) {
	b.vlog("!rng_image received in channel %s", channelID)
	img, err := b.imagesUC.GetRandom(ctx)
	if err != nil {
		b.l.Error(fmt.Errorf("msgRngImage: %w", err))
		_, _ = s.ChannelMessageSend(channelID, "Failed to get a random image.")
		return
	}
	files, err := b.downloadImages(ctx, []entity.Image{img})
	if err != nil {
		b.l.Error(fmt.Errorf("msgRngImage download: %w", err))
		return
	}
	b.channelSendFiles(s, channelID, img.AlbumName, files)
	b.vlog("!rng_image completed in channel %s: album=%q", channelID, img.AlbumName)
}

func (b *Bot) msgRngAlbum(ctx context.Context, s *discordgo.Session, channelID string) {
	b.vlog("!rng_album received in channel %s", channelID)
	imgs, err := b.imagesUC.GetRandomAlbumImages(ctx, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("msgRngAlbum: %w", err))
		_, _ = s.ChannelMessageSend(channelID, "Failed to get random album.")
		return
	}
	b.sendAlbumToChannel(ctx, s, channelID, albumNameFrom(imgs), imgs)
	b.vlog("!rng_album completed in channel %s: album=%q", channelID, albumNameFrom(imgs))
}

func (b *Bot) msgAlbum(ctx context.Context, s *discordgo.Session, channelID, albumName string) {
	b.vlog("!album received in channel %s: album=%q", channelID, albumName)
	imgs, err := b.imagesUC.GetAlbumImages(ctx, albumName, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("msgAlbum %q: %w", albumName, err))
		_, _ = s.ChannelMessageSend(channelID, fmt.Sprintf("Album %q not found or empty.", albumName))
		return
	}
	b.sendAlbumToChannel(ctx, s, channelID, albumName, imgs)
	b.vlog("!album completed in channel %s: album=%q", channelID, albumName)
}

func (b *Bot) msgFullAlbum(ctx context.Context, s *discordgo.Session, channelID, albumName string) {
	b.vlog("!full_album received in channel %s: album=%q", channelID, albumName)
	initMsg, err := s.ChannelMessageSend(channelID, fmt.Sprintf("Creating thread for album **%s**…", albumName))
	if err != nil {
		b.l.Error(fmt.Errorf("msgFullAlbum ChannelMessageSend: %w", err))
		return
	}
	thread, err := s.MessageThreadStartComplex(channelID, initMsg.ID, &discordgo.ThreadStart{
		Name:                fmt.Sprintf("Full album: %s", albumName),
		AutoArchiveDuration: 60,
		Type:                discordgo.ChannelTypeGuildPublicThread,
	})
	if err != nil {
		b.l.Error(fmt.Errorf("msgFullAlbum ThreadStart: %w", err))
		_, _ = s.ChannelMessageEdit(channelID, initMsg.ID, "Failed to create thread.")
		return
	}

	cover, hasCover, err := b.imagesUC.GetAlbumCover(ctx, albumName)
	if err != nil {
		b.l.Error(fmt.Errorf("msgFullAlbum GetAlbumCover %q: %w", albumName, err))
		return
	}
	imgs, err := b.imagesUC.GetFullAlbum(ctx, albumName)
	if err != nil {
		b.l.Error(fmt.Errorf("msgFullAlbum GetFullAlbum %q: %w", albumName, err))
		return
	}

	total := len(imgs)
	if hasCover {
		total++
	}
	b.vlog("!full_album %q: total=%d images, hasCover=%v", albumName, total, hasCover)
	b.sendFullAlbumToThread(ctx, thread.ID, albumName, cover, hasCover, imgs)

	content := fmt.Sprintf("Full album **%s** — %d images posted in <#%s>.", albumName, total, thread.ID)
	_, _ = s.ChannelMessageEdit(channelID, initMsg.ID, content)
	b.vlog("!full_album completed in channel %s: album=%q total=%d", channelID, albumName, total)
}
