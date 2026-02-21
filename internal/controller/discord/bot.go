// Package discord implements Discord bot controller (entry layer).
package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/bwmarrin/discordgo"
)

// Bot holds Discord session and dependencies for graceful start/stop.
type Bot struct {
	cfg      *config.Config
	l        logger.Interface
	uc       usecase.Translation
	imagesUC usecase.Images
	session  *discordgo.Session
	mu       sync.Mutex
	closed   bool
}

// NewBot creates a Discord bot that delegates to usecases.
func NewBot(cfg *config.Config, l logger.Interface, uc usecase.Translation, imagesUC usecase.Images) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("discord NewSession: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
	b := &Bot{cfg: cfg, l: l, uc: uc, imagesUC: imagesUC, session: s}
	s.AddHandler(b.handleReady)
	s.AddHandler(b.handleMessageCreate)
	s.AddHandler(b.handleInteractionCreate)
	return b, nil
}

// Open starts the Discord connection (non-blocking).
func (b *Bot) Open() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	return b.session.Open()
}

// Start runs Open in a goroutine so the app can continue to start HTTP and wait for signals.
func (b *Bot) Start() {
	go func() {
		if err := b.Open(); err != nil {
			b.l.Error(fmt.Errorf("discord Open: %w", err))
		}
	}()
}

// Close closes the Discord session.
func (b *Bot) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	return b.session.Close()
}

func (b *Bot) handleReady(s *discordgo.Session, r *discordgo.Ready) {
	b.l.Info("discord bot ready: user %s", r.User.Username)
	if b.cfg.Discord.ApplicationID != "" {
		cmd := &discordgo.ApplicationCommand{
			Name:        "image",
			Description: "Send a random image",
		}
		guildID := b.cfg.Discord.GuildID
		_, err := s.ApplicationCommandCreate(b.cfg.Discord.ApplicationID, guildID, cmd)
		if err != nil {
			b.l.Error(fmt.Errorf("discord register /image: %w", err))
			return
		}
		b.l.Info("registered slash command /image")
	}
}

func (b *Bot) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	content := strings.TrimSpace(strings.ToLower(m.Content))
	if content == "!ping" {
		_, _ = s.ChannelMessageSend(m.ChannelID, "pong")
		return
	}
	if content == "!image" {
		ctx := context.Background()
		image, err := b.imagesUC.GetImage(ctx)
		if err != nil {
			b.l.Error(fmt.Errorf("images GetImage: %w", err))
			_, _ = s.ChannelMessageSend(m.ChannelID, "Failed to get image.")
			return
		}
		embed := &discordgo.MessageEmbed{
			Image: &discordgo.MessageEmbedImage{URL: b.resolveImageURL(image.URL)},
			Title: "Image",
		}
		_, _ = s.ChannelMessageSendEmbed(m.ChannelID, embed)
	}
}

func (b *Bot) handleInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.ApplicationCommandData().Name != "image" {
		return
	}
	ctx := context.Background()
	image, err := b.imagesUC.GetImage(ctx)
	if err != nil {
		b.l.Error(fmt.Errorf("images GetImage: %w", err))
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Failed to get image."},
		})
		return
	}
	embed := &discordgo.MessageEmbed{
		Image: &discordgo.MessageEmbedImage{URL: b.resolveImageURL(image.URL)},
		Title: "Image",
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// resolveImageURL returns a full URL for Discord embed; if url is a path (starts with /), prepends HTTP.PublicURL.
func (b *Bot) resolveImageURL(url string) string {
	if strings.HasPrefix(url, "/") {
		return strings.TrimSuffix(b.cfg.HTTP.PublicURL, "/") + url
	}
	return url
}
