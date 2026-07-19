package discord

import (
	"context"
	"fmt"
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
		Name:        "album_mode",
		Description: "Set an album's delivery type",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type: discordgo.ApplicationCommandOptionString, Name: "name",
				Description: "Album name", Required: true,
			},
			{
				Type: discordgo.ApplicationCommandOptionString, Name: "mode",
				Description: "Delivery type", Required: true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Random", Value: "Random"},
					{Name: "Order", Value: "Order"},
					{Name: "Single", Value: "Single"},
					{Name: "Video", Value: "Video"},
					{Name: "Custom", Value: "Custom"},
				},
			},
		},
	},
	{
		Name:        "schedule",
		Description: "Manage Discord delivery rules",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "list",
				Description: "List all delivery rules",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "add",
				Description: "Add a delivery rule (Manage Channels required)",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type: discordgo.ApplicationCommandOptionString, Name: "trigger",
						Description: "When the rule fires", Required: true,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "scheduled", Value: "scheduled"},
							{Name: "new_album", Value: "new_album"},
							{Name: "new_files", Value: "new_files"},
						},
					},
					{Type: discordgo.ApplicationCommandOptionChannel, Name: "channel", Description: "Target channel", Required: true},
					{Type: discordgo.ApplicationCommandOptionString, Name: "interval", Description: `Scheduled only: Go duration, e.g. "6h"`},
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "history_size", Description: "Scheduled only: exclude this many recent albums"},
					{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Optional label"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "remove",
				Description: "Remove a delivery rule by id (Manage Channels required)",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionInteger, Name: "id", Description: "Rule id (see /schedule list)", Required: true},
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
	case "album_mode":
		b.cmdAlbumMode(s, i)
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

func (b *Bot) cmdAlbumMode(s *discordgo.Session, i *discordgo.InteractionCreate) {
	user := interactionUser(i)
	b.vlog("/album_mode received from %s", user)
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		if !b.hasSchedulePermission(i.Member) {
			b.editInteractionContent(s, i, "You need Manage Channels permission to change album mode.")
			return
		}
		albumName := ""
		modeStr := ""
		for _, opt := range i.ApplicationCommandData().Options {
			switch opt.Name {
			case "name":
				albumName = strings.TrimSpace(opt.StringValue())
			case "mode":
				modeStr = strings.TrimSpace(opt.StringValue())
			}
		}
		mode, err := entity.ParseAlbumSendMode(modeStr)
		if err != nil {
			b.editInteractionContent(s, i, fmt.Sprintf("Invalid mode %q. Choose Random, Order, Single, Video, or Custom.", modeStr))
			return
		}
		album, err := b.imagesUC.SetAlbumMode(ctx, albumName, mode)
		if err != nil {
			b.l.Error(fmt.Errorf("cmdAlbumMode SetAlbumMode %q: %w", albumName, err))
			b.editInteractionContent(s, i, fmt.Sprintf("Failed to set mode for album **%s** (not found?).", albumName))
			return
		}
		b.editInteractionContent(s, i, fmt.Sprintf("Album **%s** mode set to %s.", album.Name, album.SendMode))
		b.vlog("/album_mode completed for %s: album=%q mode=%s", user, album.Name, album.SendMode)
	}()
}

func (b *Bot) cmdSchedule(s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		data := i.ApplicationCommandData()
		if len(data.Options) == 0 {
			b.editInteractionContent(s, i, "Usage: /schedule list | add | remove")
			return
		}
		sub := data.Options[0]
		switch sub.Name {
		case "list":
			rules, err := b.rulesUC.List(ctx)
			if err != nil {
				b.l.Error(fmt.Errorf("cmdSchedule list: %w", err))
				b.editInteractionContent(s, i, "Failed to load delivery rules.")
				return
			}
			b.editInteractionContent(s, i, formatRulesList(rules))
		case "add":
			if !b.hasSchedulePermission(i.Member) {
				b.editInteractionContent(s, i, "You need Manage Channels permission to add rules.")
				return
			}
			rule := entity.DeliveryRule{GuildID: b.guildIDFromInteraction(i), Enabled: true}
			for _, opt := range sub.Options {
				switch opt.Name {
				case "trigger":
					rule.TriggerType = strings.TrimSpace(opt.StringValue())
				case "channel":
					rule.ChannelID = strings.TrimSpace(fmt.Sprintf("%v", opt.Value))
				case "interval":
					rule.SendInterval = strings.TrimSpace(opt.StringValue())
				case "history_size":
					rule.HistorySize = int(opt.IntValue())
				case "name":
					rule.Name = strings.TrimSpace(opt.StringValue())
				}
			}
			created, err := b.rulesUC.Create(ctx, rule)
			if err != nil {
				b.editInteractionContent(s, i, "Failed to add rule: "+err.Error())
				return
			}
			b.editInteractionContent(s, i, fmt.Sprintf("Added rule #%d.\n%s", created.ID, formatRuleLine(created)))
		case "remove":
			if !b.hasSchedulePermission(i.Member) {
				b.editInteractionContent(s, i, "You need Manage Channels permission to remove rules.")
				return
			}
			var id int64
			for _, opt := range sub.Options {
				if opt.Name == "id" {
					id = opt.IntValue()
				}
			}
			if err := b.rulesUC.Delete(ctx, id); err != nil {
				b.l.Error(fmt.Errorf("cmdSchedule remove %d: %w", id, err))
				b.editInteractionContent(s, i, "Failed to remove rule.")
				return
			}
			b.editInteractionContent(s, i, fmt.Sprintf("Removed rule #%d.", id))
		default:
			b.editInteractionContent(s, i, "Unknown schedule subcommand.")
		}
	}()
}

// formatRulesList renders all delivery rules as a Discord message.
func formatRulesList(rules []entity.DeliveryRule) string {
	if len(rules) == 0 {
		return "No delivery rules configured. Use /schedule add to create one."
	}
	var sb strings.Builder
	sb.WriteString("Delivery rules\n")
	for _, r := range rules {
		sb.WriteString(formatRuleLine(r))
		sb.WriteString("\n")
	}
	return sb.String()
}

// formatRuleLine renders one delivery rule as a single line.
func formatRuleLine(r entity.DeliveryRule) string {
	status := "on"
	if !r.Enabled {
		status = "off"
	}
	label := r.Name
	if label == "" {
		label = "(unnamed)"
	}
	extra := ""
	if r.TriggerType == entity.TriggerScheduled {
		extra = fmt.Sprintf(" every %s, history %d", r.SendInterval, r.HistorySize)
	}
	return fmt.Sprintf("• #%d [%s] %s → <#%s>%s (%s)", r.ID, r.TriggerType, label, r.ChannelID, extra, status)
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
