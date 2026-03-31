package discord

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

var slashCommands = []*discordgo.ApplicationCommand{
	{Name: "image", Description: "Send the default image"},
	{Name: "rng_image", Description: "Send one random image from all albums"},
	{Name: "rng_album", Description: "Send 10 random images from a random album"},
	{
		Name:        "album",
		Description: "Send 10 random images from a named album",
		Options: []*discordgo.ApplicationCommandOption{{
			Type: discordgo.ApplicationCommandOptionString, Name: "name",
			Description: "Album name", Required: true,
		}},
	},
	{
		Name:        "full_album",
		Description: "Send all images in an album via a thread (10 at a time)",
		Options: []*discordgo.ApplicationCommandOption{{
			Type: discordgo.ApplicationCommandOptionString, Name: "name",
			Description: "Album name", Required: true,
		}},
	},
	{
		Name:        "schedule",
		Description: "Show or update scheduled send settings",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "show",
				Description: "Show current effective schedule settings",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set",
				Description: "Update schedule settings (Manage Channels required)",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Target channel", Required: true},
					{Type: discordgo.ApplicationCommandOptionString, Name: "interval", Description: `Go duration, e.g. "6h"`, Required: true},
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "history_size", Description: "Exclude this many recent albums", Required: true},
				},
			},
		},
	},
}

func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	b.l.Info("discord bot ready: user %s", r.User.Username)
	if b.cfg.Discord.ApplicationID == "" {
		b.l.Info("DISCORD_APPLICATION_ID not set, skipping slash command registration")
		return
	}
	registered, err := s.ApplicationCommandBulkOverwrite(
		b.cfg.Discord.ApplicationID, b.cfg.Discord.GuildID, slashCommands,
	)
	if err != nil {
		b.l.Error(fmt.Errorf("discord BulkOverwrite commands: %w", err))
		return
	}
	for _, cmd := range registered {
		b.l.Info("registered slash command /%s", cmd.Name)
	}
}

func (b *Bot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	switch i.ApplicationCommandData().Name {
	case "image":
		b.cmdImage(s, i)
	case "rng_image":
		b.cmdRngImage(s, i)
	case "rng_album":
		b.cmdRngAlbum(s, i)
	case "album":
		b.cmdAlbum(s, i)
	case "full_album":
		b.cmdFullAlbum(s, i)
	case "schedule":
		b.cmdSchedule(s, i)
	}
}

func (b *Bot) cmdImage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	user := interactionUser(i)
	b.vlog("/image received from %s", user)
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		img, err := b.imagesUC.GetImage(ctx)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdImage: %w", err))
			b.editInteractionContent(s, i, "Failed to get image.")
			return
		}
		files, err := b.downloadImages(ctx, []entity.Image{img})
		if err != nil {
			b.l.Error(fmt.Errorf("cmdImage download: %w", err))
			b.editInteractionContent(s, i, "Failed to download image.")
			return
		}
		b.editInteractionFiles(s, i, "", files)
		b.vlog("/image completed for %s", user)
	}()
}

func (b *Bot) cmdRngImage(s *discordgo.Session, i *discordgo.InteractionCreate) {
	user := interactionUser(i)
	b.vlog("/rng_image received from %s", user)
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		img, err := b.imagesUC.GetRandom(ctx)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdRngImage: %w", err))
			b.editInteractionContent(s, i, "Failed to get a random image.")
			return
		}
		files, err := b.downloadImages(ctx, []entity.Image{img})
		if err != nil {
			b.l.Error(fmt.Errorf("cmdRngImage download: %w", err))
			b.editInteractionContent(s, i, "Failed to download image.")
			return
		}
		b.editInteractionFiles(s, i, img.AlbumName, files)
		b.vlog("/rng_image completed for %s: album=%q", user, img.AlbumName)
	}()
}

func (b *Bot) cmdRngAlbum(s *discordgo.Session, i *discordgo.InteractionCreate) {
	user := interactionUser(i)
	b.vlog("/rng_album received from %s", user)
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		imgs, err := b.imagesUC.GetRandomAlbumImages(ctx, albumPoolSize)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdRngAlbum: %w", err))
			b.editInteractionContent(s, i, "Failed to get random album.")
			return
		}
		files, err := b.downloadAndFit(ctx, imgs)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdRngAlbum downloadAndFit: %w", err))
			b.editInteractionContent(s, i, "Failed to download images.")
			return
		}
		b.editInteractionFiles(s, i, albumNameFrom(imgs), files)
		b.vlog("/rng_album completed for %s: album=%q files=%d", user, albumNameFrom(imgs), len(files))
	}()
}

func (b *Bot) cmdAlbum(s *discordgo.Session, i *discordgo.InteractionCreate) {
	albumName := i.ApplicationCommandData().Options[0].StringValue()
	user := interactionUser(i)
	b.vlog("/album received from %s: album=%q", user, albumName)
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		imgs, err := b.imagesUC.GetAlbumImages(ctx, albumName, albumPoolSize)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdAlbum %q: %w", albumName, err))
			b.editInteractionContent(s, i, fmt.Sprintf("Album %q not found or empty.", albumName))
			return
		}
		files, err := b.downloadAndFit(ctx, imgs)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdAlbum downloadAndFit %q: %w", albumName, err))
			b.editInteractionContent(s, i, "Failed to download images.")
			return
		}
		b.editInteractionFiles(s, i, albumName, files)
		b.vlog("/album completed for %s: album=%q files=%d", user, albumName, len(files))
	}()
}

func (b *Bot) cmdFullAlbum(s *discordgo.Session, i *discordgo.InteractionCreate) {
	albumName := i.ApplicationCommandData().Options[0].StringValue()
	user := interactionUser(i)
	b.vlog("/full_album received from %s: album=%q", user, albumName)
	b.deferInteraction(s, i)

	go func() {
		ctx := context.Background()

		cover, hasCover, err := b.imagesUC.GetAlbumCover(ctx, albumName)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdFullAlbum GetAlbumCover %q: %w", albumName, err))
			b.editInteractionContent(s, i, fmt.Sprintf("Album **%s** not found.", albumName))
			return
		}
		imgs, err := b.imagesUC.GetFullAlbum(ctx, albumName)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdFullAlbum GetFullAlbum %q: %w", albumName, err))
			b.editInteractionContent(s, i, fmt.Sprintf("Album **%s** not found.", albumName))
			return
		}

		total := len(imgs)
		if hasCover {
			total++
		}
		if total == 0 {
			b.editInteractionContent(s, i, fmt.Sprintf("Album **%s** is empty.", albumName))
			return
		}
		b.vlog("/full_album %q: total=%d images, hasCover=%v", albumName, total, hasCover)

		msg, err := b.session.InteractionResponse(i.Interaction)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdFullAlbum InteractionResponse: %w", err))
			return
		}
		thread, err := b.session.MessageThreadStartComplex(msg.ChannelID, msg.ID, &discordgo.ThreadStart{
			Name:                fmt.Sprintf("Full album: %s", albumName),
			AutoArchiveDuration: 60,
			Type:                discordgo.ChannelTypeGuildPublicThread,
		})
		if err != nil {
			b.l.Error(fmt.Errorf("cmdFullAlbum ThreadStart: %w", err))
			b.editInteractionContent(s, i, "Failed to create thread.")
			return
		}

		b.sendFullAlbumToThread(ctx, thread.ID, albumName, cover, hasCover, imgs)
		b.editInteractionContent(s, i,
			fmt.Sprintf("Full album **%s** — %d images posted in <#%s>.", albumName, total, thread.ID))
		b.vlog("/full_album completed for %s: album=%q total=%d", user, albumName, total)
	}()
}

func (b *Bot) cmdSchedule(s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		data := i.ApplicationCommandData()
		if len(data.Options) == 0 {
			b.editInteractionContent(s, i, "Usage: /schedule show or /schedule set")
			return
		}
		sub := data.Options[0]
		guildID := b.guildIDFromInteraction(i)
		switch sub.Name {
		case "show":
			effective, err := b.settingsUC.GetEffectiveSchedule(ctx, guildID)
			if err != nil {
				b.l.Error(fmt.Errorf("cmdSchedule show: %w", err))
				b.editInteractionContent(s, i, "Failed to load schedule settings.")
				return
			}
			b.editInteractionContent(s, i, b.scheduleDisplay(effective))
		case "set":
			if !b.hasSchedulePermission(i.Member) {
				b.editInteractionContent(s, i, "You need Manage Channels permission to update schedule.")
				return
			}
			channelID := ""
			interval := ""
			historySize := 0
			for _, opt := range sub.Options {
				switch opt.Name {
				case "channel":
					channelID = strings.TrimSpace(fmt.Sprintf("%v", opt.Value))
				case "interval":
					interval = strings.TrimSpace(opt.StringValue())
				case "history_size":
					historySize = int(opt.IntValue())
				}
			}
			if _, err := timeParseDuration(interval); err != nil {
				b.editInteractionContent(s, i, "Invalid interval. Example: 6h, 30m, 24h.")
				return
			}
			if historySize <= 0 {
				b.editInteractionContent(s, i, "history_size must be > 0.")
				return
			}
			if _, err := b.settingsUC.UpsertSchedule(ctx, entity.DiscordScheduleSettings{
				GuildID:         guildID,
				SendChannelID:   channelID,
				SendInterval:    interval,
				SendHistorySize: historySize,
			}); err != nil {
				b.l.Error(fmt.Errorf("cmdSchedule set: %w", err))
				b.editInteractionContent(s, i, "Failed to update schedule settings.")
				return
			}
			effective, err := b.settingsUC.GetEffectiveSchedule(ctx, guildID)
			if err != nil {
				b.l.Error(fmt.Errorf("cmdSchedule set effective: %w", err))
				b.editInteractionContent(s, i, "Updated, but failed to reload effective settings.")
				return
			}
			b.editInteractionContent(s, i, "Schedule updated.\n"+b.scheduleDisplay(effective))
		default:
			b.editInteractionContent(s, i, "Unknown schedule subcommand.")
		}
	}()
}

func (b *Bot) hasSchedulePermission(member *discordgo.Member) bool {
	if member == nil {
		return false
	}
	perms := member.Permissions
	return perms&discordgo.PermissionManageChannels != 0 || perms&discordgo.PermissionAdministrator != 0
}

func (b *Bot) guildIDFromInteraction(i *discordgo.InteractionCreate) string {
	if i.GuildID != "" {
		return i.GuildID
	}
	if b.cfg.Discord.GuildID != "" {
		return b.cfg.Discord.GuildID
	}
	return ""
}

func (b *Bot) scheduleDisplay(e entity.EffectiveScheduleSettings) string {
	return "Schedule settings\n" +
		"- guild: `" + e.GuildID + "`\n" +
		"- channel: `<#" + e.SendChannelID + ">` (" + e.SourceSendChannelID + ")\n" +
		"- interval: `" + e.SendInterval + "` (" + e.SourceSendInterval + ")\n" +
		"- history_size: `" + strconv.Itoa(e.SendHistorySize) + "` (" + e.SourceSendHistorySize + ")"
}
