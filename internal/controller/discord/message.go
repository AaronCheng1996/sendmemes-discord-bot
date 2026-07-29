package discord

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

// captionValues carries everything a placeholder can expand to for one send.
// Callers fill in what they know; zero values simply render as "0"/"".
type captionValues struct {
	Album     entity.Album
	Sent      int // items in this message
	Total     int // items available
	NewImages int // discovery notifications only
	NewVideos int
	Prefix    string // "[TEST] " for admin test sends
}

// renderCaption fills tmpl's placeholders from v. An unrecognized placeholder is
// left as-is (strings.NewReplacer only touches known tokens — no template
// engine, so there is nothing else for it to do). An empty (or all-whitespace)
// tmpl falls back to the caption used before embeds were introduced, keeping
// rules that have never set a template looking the same as before.
func renderCaption(tmpl string, v captionValues) string {
	if strings.TrimSpace(tmpl) == "" {
		return defaultCaption(v)
	}
	now := time.Now()
	replacer := strings.NewReplacer(
		"{album}", v.Album.Name,
		"{album_id}", strconv.Itoa(v.Album.ID),
		"{mode}", string(v.Album.SendMode),
		"{count}", strconv.Itoa(v.Sent),
		"{total}", strconv.Itoa(v.Total),
		"{rating}", strconv.Itoa(v.Album.PositiveRating),
		"{new_images}", strconv.Itoa(v.NewImages),
		"{new_videos}", strconv.Itoa(v.NewVideos),
		"{new_total}", strconv.Itoa(v.NewImages+v.NewVideos),
		"{prefix}", v.Prefix,
		"{date}", now.Format("2006-01-02"),
		"{time}", now.Format("15:04"),
	)
	return replacer.Replace(tmpl)
}

// defaultCaption is the pre-embed caption text: the album name (with optional
// prefix like "[TEST] "), plus a "(showing X of Y)" suffix when the album has
// more content than fit in this send.
func defaultCaption(v captionValues) string {
	if v.Total > 0 && v.Sent > 0 && v.Sent < v.Total {
		return fmt.Sprintf("%s%s (showing %d of %d)", v.Prefix, v.Album.Name, v.Sent, v.Total)
	}
	return v.Prefix + v.Album.Name
}

// renderedMessage is a message-style layer stack resolved against one send:
// placeholders filled in, embed preference settled.
type renderedMessage struct {
	// Style is the merged style this message came from, kept so the embed
	// builder can read the non-text options (colour, toggles, link).
	Style    entity.MessageStyle
	UseEmbed bool
	// Title is empty when no layer set one; embeds then fall back to the album
	// name and plain messages omit the headline entirely.
	Title  string
	Body   string
	Footer string // empty = default "album #12 · Random"
	Author string // empty = no author line
}

// renderMessage resolves the merged style for a single send. Title and Body
// share the same placeholder set, so a rule can set a common headline while an
// album supplies its own body.
func renderMessage(style entity.MessageStyle, v captionValues) renderedMessage {
	out := renderedMessage{
		Style:    style,
		UseEmbed: style.EmbedEnabled(),
		Body:     renderCaption(style.Body, v),
	}
	// Title/footer/author are optional: an empty template means "no override",
	// not "render the default caption", so they are only expanded when set.
	if strings.TrimSpace(style.Title) != "" {
		out.Title = renderCaption(style.Title, v)
	}
	if strings.TrimSpace(style.Footer) != "" {
		out.Footer = renderCaption(style.Footer, v)
	}
	if strings.TrimSpace(style.Author) != "" {
		out.Author = renderCaption(style.Author, v)
	}
	return out
}

// plainContent assembles a non-embed message: the headline (bold, when set) on
// its own line above the body.
func plainContent(m renderedMessage) string {
	switch {
	case m.Title != "" && m.Body != "":
		return "**" + m.Title + "**\n" + m.Body
	case m.Title != "":
		return "**" + m.Title + "**"
	default:
		return m.Body
	}
}

// albumEmbed builds the shared embed shape for album deliveries: title is the
// resolved headline (falling back to the album name), description is the
// rendered body, footer identifies the album and its send mode, and thumbURL
// (when non-empty) becomes the small corner thumbnail. Callers that attach
// files may additionally set Image to "attachment://<filename>" so the first
// file renders large inside the embed.
func albumEmbed(album entity.Album, thumbURL string, m renderedMessage) *discordgo.MessageEmbed {
	title := m.Title
	if title == "" {
		title = album.Name
	}

	footer := m.Footer
	if footer == "" {
		footer = fmt.Sprintf("album #%d", album.ID)
		if album.SendMode != "" {
			footer += " · " + string(album.SendMode)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: m.Body,
		Color:       resolveEmbedColor(m.Style.Color, album.SendMode),
		Footer:      &discordgo.MessageEmbedFooter{Text: footer},
	}
	if m.Style.URL != "" {
		embed.URL = m.Style.URL
	}
	if m.Author != "" {
		embed.Author = &discordgo.MessageEmbedAuthor{Name: m.Author}
	}
	if m.Style.TimestampEnabled() {
		embed.Timestamp = time.Now().Format(time.RFC3339)
	}
	if thumbURL != "" && m.Style.ThumbnailEnabled() {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: thumbURL}
	}
	return embed
}

// resolveEmbedColor parses a "#rrggbb" override, falling back to the send
// mode's accent colour when it is empty or malformed.
func resolveEmbedColor(hex string, mode entity.AlbumSendMode) int {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if hex == "" {
		return embedColor(mode)
	}
	v, err := strconv.ParseInt(hex, 16, 32)
	if err != nil || v < 0 {
		return embedColor(mode)
	}
	return int(v)
}

// sendStyled delivers one album message in whichever shape the resolved style
// asked for: a rich embed, or a plain text message with the same title/body.
// Attachments and components (e.g. the Full-album button) work either way.
//
// embedImage names the attachment to render large inside the embed; it is
// ignored in plain mode, where Discord previews attachments on its own.
func (b *Bot) sendStyled(
	channelID string,
	album entity.Album,
	m renderedMessage,
	thumbURL, embedImage string,
	files []*discordgo.File,
	components []discordgo.MessageComponent,
) *discordgo.Message {
	if !m.UseEmbed {
		return b.channelSendPlain(channelID, plainContent(m), files, components)
	}
	embed := albumEmbed(album, thumbURL, m)
	if embedImage != "" && m.Style.ImageEnabled() {
		embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://" + embedImage}
	}
	return b.channelSendEmbed(channelID, embed, files, components)
}

// channelSendPlain is the non-embed counterpart of channelSendEmbed: same
// attachments, components and auto-reaction, just no embed wrapper.
func (b *Bot) channelSendPlain(channelID, content string, files []*discordgo.File, components []discordgo.MessageComponent) *discordgo.Message {
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:    content,
		Files:      files,
		Components: components,
	})
	if err != nil {
		b.l.Error(fmt.Errorf("channelSendPlain: %w", err))
		return nil
	}
	b.autoReact(channelID, msg.ID)
	return msg
}

// autoReact adds the discoverability reaction to a post the bot just made.
func (b *Bot) autoReact(channelID, messageID string) {
	if !autoReactOnPost {
		return
	}
	if err := b.session.MessageReactionAdd(channelID, messageID, autoReactEmoji); err != nil {
		b.l.Error(fmt.Errorf("autoReact: %w", err))
	}
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
	b.autoReact(channelID, msg.ID)
	return msg
}
