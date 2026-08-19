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

// testMarker prefixes admin previews so they can never be mistaken for a real
// post. It is applied automatically rather than through a placeholder: relying
// on templates to include one meant forgetting it silently produced posts
// indistinguishable from the scheduled ones.
const testMarker = "[TEST] "

// sendContext describes the delivery a message belongs to, as opposed to how it
// is styled. It supplies the context-dependent placeholders.
type sendContext struct {
	Test      bool   // admin preview
	RuleName  string // delivery rule that triggered this send, if any
	ChannelID string
}

// timeTokens are available in every context.
func timeTokens() map[string]string {
	now := time.Now()
	return map[string]string{
		"{date}":     now.Format("2006-01-02"),
		"{time}":     now.Format("15:04"),
		"{datetime}": now.Format("2006-01-02 15:04"),
		"{weekday}":  now.Format("Monday"),
	}
}

// albumCounts is how much media an album actually holds — what people mean by
// "total", as opposed to {shown}, the number of files in one message. Known is
// false when the lookup failed, in which case the tokens are left unexpanded
// rather than reporting a confident zero.
type albumCounts struct {
	Images int
	Videos int
	Known  bool
}

// imagesOr/videosOr fall back to a caller-supplied count (usually the size of
// the listing it already fetched) when the album lookup failed.
func (c albumCounts) imagesOr(fallback int) int {
	if c.Known {
		return c.Images
	}
	return fallback
}

func (c albumCounts) videosOr(fallback int) int {
	if c.Known {
		return c.Videos
	}
	return fallback
}

// addAlbumCountTokens exposes the album's real contents. Both scheduled sends
// and discovery notifications use it, so a caption that reports a running total
// works the same either way.
func addAlbumCountTokens(tokens map[string]string, counts albumCounts) {
	if !counts.Known {
		return
	}
	tokens["{album_images}"] = strconv.Itoa(counts.Images)
	tokens["{album_videos}"] = strconv.Itoa(counts.Videos)
	tokens["{album_total}"] = strconv.Itoa(counts.Images + counts.Videos)
}

// albumTokens describes one album delivery.
func albumTokens(album entity.Album, shown int, counts albumCounts, sc sendContext) map[string]string {
	tokens := timeTokens()
	tokens["{album}"] = album.Name
	tokens["{album_id}"] = strconv.Itoa(album.ID)
	tokens["{mode}"] = string(album.SendMode)
	tokens["{rating}"] = strconv.Itoa(album.PositiveRating)
	tokens["{shown}"] = strconv.Itoa(shown)
	if album.LastSentAt != nil {
		tokens["{last_sent}"] = album.LastSentAt.Format("2006-01-02 15:04")
	}
	addAlbumCountTokens(tokens, counts)
	addContextTokens(tokens, sc)
	return tokens
}

// discoveryTokens describes a sync notification: what the sync just found, plus
// the album totals those files landed in. The album's send mode and rating are
// absent because a SyncEvent carries only the album's id and name.
func discoveryTokens(album entity.Album, ev entity.SyncEvent, shown int, counts albumCounts, sc sendContext) map[string]string {
	tokens := timeTokens()
	tokens["{album}"] = album.Name
	tokens["{album_id}"] = strconv.Itoa(album.ID)
	tokens["{shown}"] = strconv.Itoa(shown)
	tokens["{new_images}"] = strconv.Itoa(ev.NewImages)
	tokens["{new_videos}"] = strconv.Itoa(ev.NewVideos)
	tokens["{new_total}"] = strconv.Itoa(ev.NewImages + ev.NewVideos)
	addAlbumCountTokens(tokens, counts)
	addContextTokens(tokens, sc)
	return tokens
}

func addContextTokens(tokens map[string]string, sc sendContext) {
	if sc.RuleName != "" {
		tokens["{rule}"] = sc.RuleName
	}
	if sc.ChannelID != "" {
		tokens["{channel}"] = "<#" + sc.ChannelID + ">"
	}
}

// renderCaption substitutes the placeholders tokens knows about. Anything else —
// a typo, or a placeholder that belongs to a different context (e.g. {new_images}
// in a scheduled send) — is deliberately left verbatim, so a wrong placeholder
// looks wrong instead of quietly rendering "0". An empty template falls back to
// the built-in caption.
func renderCaption(tmpl string, tokens map[string]string, fallback string) string {
	if strings.TrimSpace(tmpl) == "" {
		return fallback
	}
	pairs := make([]string, 0, len(tokens)*2)
	for k, v := range tokens {
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(tmpl)
}

// defaultCaption is the built-in body: the album name plus a "(showing X of Y)"
// suffix when the album holds more than this message could carry.
func defaultCaption(album entity.Album, shown, total int) string {
	if total > 0 && shown > 0 && shown < total {
		return fmt.Sprintf("%s (showing %d of %d)", album.Name, shown, total)
	}
	return album.Name
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
	// Test marks an admin preview; the marker is added when the final headline
	// is assembled, so it shows whether or not a custom title was configured.
	Test bool
}

// headline resolves the visible title, applying the test marker.
func (m renderedMessage) headline(fallback string) string {
	title := m.Title
	if title == "" {
		title = fallback
	}
	if m.Test {
		title = testMarker + title
	}
	return title
}

// renderMessage resolves the merged style for a single send. Title and Body
// share the same placeholder set, so a rule can set a common headline while an
// album supplies its own body.
func renderMessage(style entity.MessageStyle, tokens map[string]string, fallbackBody string, test bool) renderedMessage {
	out := renderedMessage{
		Style:    style,
		UseEmbed: style.EmbedEnabled(),
		Test:     test,
		Body:     renderCaption(style.Body, tokens, fallbackBody),
	}
	// Title/footer/author are optional: an empty template means "no override",
	// not "render the default caption", so they expand only when set.
	if strings.TrimSpace(style.Title) != "" {
		out.Title = renderCaption(style.Title, tokens, "")
	}
	if strings.TrimSpace(style.Footer) != "" {
		out.Footer = renderCaption(style.Footer, tokens, "")
	}
	if strings.TrimSpace(style.Author) != "" {
		out.Author = renderCaption(style.Author, tokens, "")
	}
	return out
}

// plainContent assembles a non-embed message: the headline (bold, when set) on
// its own line above the body.
func plainContent(m renderedMessage) string {
	// With no configured title a plain message is just the body, so the test
	// marker goes in front of the text instead of an invented headline.
	if m.Title == "" {
		if m.Test {
			return testMarker + m.Body
		}
		return m.Body
	}
	head := "**" + m.headline("") + "**"
	if m.Body == "" {
		return head
	}
	return head + "\n" + m.Body
}

// albumEmbed builds the shared embed shape for album deliveries: title is the
// resolved headline (falling back to the album name), description is the
// rendered body, footer identifies the album and its send mode, and thumbURL
// (when non-empty) becomes the small corner thumbnail. Callers that attach
// files may additionally set Image to "attachment://<filename>" so the first
// file renders large inside the embed.
func albumEmbed(album entity.Album, thumbURL string, m renderedMessage) *discordgo.MessageEmbed {
	title := m.headline(album.Name)

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
	files []fileEntry,
	components []discordgo.MessageComponent,
) *discordgo.Message {
	what := albumLabel(album)
	if !m.UseEmbed {
		return b.channelSendPlain(channelID, plainContent(m), what, files, components)
	}
	embed := albumEmbed(album, thumbURL, m)
	if embedImage != "" && m.Style.ImageEnabled() {
		embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://" + embedImage}
	}
	return b.channelSendEmbed(channelID, embed, what, files, components)
}

// channelSendPlain is the non-embed counterpart of channelSendEmbed: same
// attachments, components and auto-reaction, just no embed wrapper.
func (b *Bot) channelSendPlain(channelID, content, what string, files []fileEntry, components []discordgo.MessageComponent) *discordgo.Message {
	msg := b.sendWithBackoff(channelID, what, files, &discordgo.MessageSend{
		Content:    content,
		Components: components,
	})
	if msg != nil {
		b.autoReact(channelID, msg.ID)
	}
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
func (b *Bot) channelSendEmbed(channelID string, embed *discordgo.MessageEmbed, what string, files []fileEntry, components []discordgo.MessageComponent) *discordgo.Message {
	msg := b.sendWithBackoff(channelID, what, files, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
	if msg != nil {
		b.autoReact(channelID, msg.ID)
	}
	return msg
}
