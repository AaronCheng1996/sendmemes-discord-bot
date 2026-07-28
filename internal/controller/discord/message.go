package discord

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

// embedColorByMode gives each send mode a distinct accent color so scanning a
// busy channel makes it obvious what kind of post this is at a glance.
var embedColorByMode = map[entity.AlbumSendMode]int{
	entity.AlbumSendModeRandom: 0x5390ff,
	entity.AlbumSendModeOrder:  0x9b59b6,
	entity.AlbumSendModeSingle: 0x2ecc71,
	entity.AlbumSendModeVideo:  0xe67e22,
	entity.AlbumSendModeCustom: 0x1abc9c,
}

const embedColorDefault = 0x5390ff

// autoReactOnPost controls whether the bot adds a 👍 reaction to every embed
// post it sends. The reaction itself already counts toward an album's
// positive_rating (see handleReactionAdd), but nothing told users that
// reacting at all was useful — this makes the mechanic discoverable. Kept as
// a constant switch rather than a DB-backed setting since flipping it is a
// deploy-time/code change, not something that needs runtime UI.
const autoReactOnPost = true

// autoReactEmoji is the reaction the bot adds to its own posts when
// autoReactOnPost is enabled.
const autoReactEmoji = "👍"

// embedColor returns the accent color for mode, falling back to a neutral
// default for unrecognized/empty modes (e.g. a sync-event embed with no album).
func embedColor(mode entity.AlbumSendMode) int {
	if c, ok := embedColorByMode[mode]; ok {
		return c
	}
	return embedColorDefault
}

// renderCaption fills tmpl's placeholders with per-send values:
// {album} {count} {total} {rating} {prefix}. An unrecognized placeholder is
// left as-is (strings.NewReplacer only touches known tokens — no template
// engine, so there is nothing else for it to do). An empty (or all-whitespace)
// tmpl falls back to the caption used before embeds were introduced, keeping
// rules that have never set a template looking the same as before.
func renderCaption(tmpl string, album entity.Album, sent, total int, prefix string) string {
	if strings.TrimSpace(tmpl) == "" {
		return defaultCaption(album, sent, total, prefix)
	}
	replacer := strings.NewReplacer(
		"{album}", album.Name,
		"{count}", strconv.Itoa(sent),
		"{total}", strconv.Itoa(total),
		"{rating}", strconv.Itoa(album.PositiveRating),
		"{prefix}", prefix,
	)
	return replacer.Replace(tmpl)
}

// defaultCaption is the pre-embed caption text: the album name (with optional
// prefix like "[TEST] "), plus a "(showing X of Y)" suffix when the album has
// more content than fit in this send.
func defaultCaption(album entity.Album, sent, total int, prefix string) string {
	if total > 0 && sent > 0 && sent < total {
		return fmt.Sprintf("%s%s (showing %d of %d)", prefix, album.Name, sent, total)
	}
	return prefix + album.Name
}

// albumEmbed builds the shared embed shape for album deliveries: title is the
// album name, description is the (already-rendered) caption, footer identifies
// the album and its send mode, and thumbURL (when non-empty) becomes the
// small corner thumbnail. Callers that attach files may additionally set
// Image to "attachment://<filename>" so the first file renders large inside
// the embed.
func albumEmbed(album entity.Album, thumbURL, description string) *discordgo.MessageEmbed {
	footer := fmt.Sprintf("album #%d", album.ID)
	if album.SendMode != "" {
		footer += " · " + string(album.SendMode)
	}
	embed := &discordgo.MessageEmbed{
		Title:       album.Name,
		Description: description,
		Color:       embedColor(album.SendMode),
		Footer:      &discordgo.MessageEmbedFooter{Text: footer},
	}
	if thumbURL != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: thumbURL}
	}
	return embed
}

// channelSendEmbed sends embed alongside optional file attachments and
// message components (e.g. the Full-album button). Returns the sent message
// (nil on failure).
func (b *Bot) channelSendEmbed(channelID string, embed *discordgo.MessageEmbed, files []*discordgo.File, components []discordgo.MessageComponent) *discordgo.Message {
	payload := &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Files:      files,
		Components: components,
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, payload)
	if err != nil {
		b.l.Error(fmt.Errorf("channelSendEmbed: %w", err))
		return nil
	}
	if autoReactOnPost {
		if rerr := b.session.MessageReactionAdd(channelID, msg.ID, autoReactEmoji); rerr != nil {
			b.l.Error(fmt.Errorf("channelSendEmbed auto-react: %w", rerr))
		}
	}
	return msg
}
