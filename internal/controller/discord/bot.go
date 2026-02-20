// Package discord implements Discord bot controller (entry layer).
package discord

import (
	"fmt"
	"sync"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
	"github.com/bwmarrin/discordgo"
)

// Bot holds Discord session and dependencies for graceful start/stop.
type Bot struct {
	cfg    *config.Config
	l      logger.Interface
	uc     usecase.Translation
	session *discordgo.Session
	mu     sync.Mutex
	closed bool
}

// NewBot creates a Discord bot that delegates to usecase.
func NewBot(cfg *config.Config, l logger.Interface, uc usecase.Translation) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("discord NewSession: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages
	b := &Bot{cfg: cfg, l: l, uc: uc, session: s}
	s.AddHandler(b.handleReady)
	s.AddHandler(b.handleMessageCreate)
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
}

func (b *Bot) handleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	// Optional: delegate to usecase (e.g. translate, meme) or respond to !ping
	if m.Content == "!ping" {
		_, _ = s.ChannelMessageSend(m.ChannelID, "pong")
	}
}
