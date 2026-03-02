// Package discord implements Discord bot controller (entry layer).
package discord

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/AaronCheng1996/sendmemes-discord-bot/config"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/usecase"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/logger"
)

const (
	albumBatchSize = 10 // target images per Discord message / thread post
	// albumPoolSize is the number of images fetched upfront so that after
	// size-based trimming we still have candidates to refill back to albumBatchSize.
	albumPoolSize = albumBatchSize * 2

	// discordMsgLimit is the safe total file-size limit per Discord message.
	// Discord's hard cap is 25 MB; we use 24 MB to leave room for JSON overhead.
	discordMsgLimit = 24 * 1024 * 1024

	// maxFillAttempts: after trimming oversized images, try this many times to
	// randomly add a replacement before giving up and sending the current set.
	maxFillAttempts = 3

	// downloadTimeout is used for both pCloud downloads and Discord uploads.
	// Large albums can have multi-MB images; give plenty of headroom.
	downloadTimeout = 5 * time.Minute

	// reactMapMaxSize is the maximum number of scheduled-send messages tracked
	// for reaction-based feedback.  Oldest entries are evicted when full.
	reactMapMaxSize = 200
)

// fileEntry is an already-downloaded image file, kept in memory so that
// fitToLimit can inspect sizes and reassemble the final Discord file list
// without extra network round-trips.
type fileEntry struct {
	data    []byte
	name    string
	isCover bool
}

func (f fileEntry) size() int { return len(f.data) }

// fitToLimit selects files from pool to send as one Discord message.
//
// Strategy (per spec):
//  1. Start with cover (always kept) + up to targetCount−1 non-cover images.
//  2. While total size > maxBytes: remove the largest non-cover image.
//  3. While count < targetCount and pool has candidates:
//     – pick a random candidate; add it if it fits.
//     – after maxFillAttempts consecutive failures, stop.
func fitToLimit(pool []fileEntry, targetCount, maxBytes int) []fileEntry {
	if len(pool) == 0 {
		return nil
	}

	// Partition: cover (first match) vs. non-cover candidates.
	var cover *fileEntry
	candidates := make([]fileEntry, 0, len(pool))
	for i := range pool {
		if pool[i].isCover && cover == nil {
			cp := pool[i]
			cover = &cp
		} else {
			candidates = append(candidates, pool[i])
		}
	}

	// Build initial selection.
	selected := make([]fileEntry, 0, targetCount)
	totalBytes := 0
	if cover != nil {
		selected = append(selected, *cover)
		totalBytes += cover.size()
	}
	remaining := make([]fileEntry, 0, len(candidates))
	for _, fe := range candidates {
		if len(selected) < targetCount {
			selected = append(selected, fe)
			totalBytes += fe.size()
		} else {
			remaining = append(remaining, fe)
		}
	}

	// Step 2: trim largest non-cover until within limit.
	for totalBytes > maxBytes {
		maxIdx, maxSz := -1, 0
		for j, fe := range selected {
			if !fe.isCover && fe.size() > maxSz {
				maxSz = fe.size()
				maxIdx = j
			}
		}
		if maxIdx == -1 {
			break // only cover left — nothing safe to remove
		}
		removed := selected[maxIdx]
		totalBytes -= removed.size()
		remaining = append(remaining, removed)
		selected = append(selected[:maxIdx], selected[maxIdx+1:]...)
	}

	// Step 3: refill from remaining pool (random order, up to maxFillAttempts consecutive failures).
	consecutiveFails := 0
	for len(selected) < targetCount && len(remaining) > 0 && consecutiveFails < maxFillAttempts {
		idx := rand.Intn(len(remaining))
		candidate := remaining[idx]
		// Remove this candidate from the pool regardless of outcome.
		remaining[idx] = remaining[len(remaining)-1]
		remaining = remaining[:len(remaining)-1]

		if totalBytes+candidate.size() <= maxBytes {
			selected = append(selected, candidate)
			totalBytes += candidate.size()
			consecutiveFails = 0
		} else {
			consecutiveFails++
		}
	}

	return selected
}

// entriesToFiles converts fileEntry slice to discordgo.File slice.
func entriesToFiles(entries []fileEntry) []*discordgo.File {
	files := make([]*discordgo.File, 0, len(entries))
	for _, fe := range entries {
		files = append(files, &discordgo.File{
			Name:   fe.name,
			Reader: bytes.NewReader(fe.data),
		})
	}
	return files
}

// ---------------------------------------------------------------------------
// Bot
// ---------------------------------------------------------------------------

// Bot holds Discord session and dependencies for graceful start/stop.
type Bot struct {
	cfg        *config.Config
	l          logger.Interface
	uc         usecase.Translation
	imagesUC   usecase.Images
	syncUC     usecase.Sync
	session    *discordgo.Session
	httpClient *http.Client
	mu         sync.Mutex
	closed     bool
	stopCh     chan struct{}

	// Reaction-feedback tracking for scheduled-send messages.
	// reactMap holds messageID → albumID for the most recent reactMapMaxSize sends.
	// reactQueue is a FIFO used to evict the oldest entry when the map is full.
	reactMu    sync.RWMutex
	reactMap   map[string]int
	reactQueue []string
}

// NewBot creates a Discord bot that delegates to use cases.
func NewBot(
	cfg *config.Config,
	l logger.Interface,
	uc usecase.Translation,
	imagesUC usecase.Images,
	syncUC usecase.Sync,
) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("discord NewSession: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsGuildMessageReactions

	// discordgo defaults to 20 s — far too short for uploading many large images.
	s.Client = &http.Client{Timeout: downloadTimeout}

	b := &Bot{
		cfg:      cfg,
		l:        l,
		uc:       uc,
		imagesUC: imagesUC,
		syncUC:   syncUC,
		session:  s,
		// Separate client for pCloud downloads (same generous timeout).
		httpClient: &http.Client{Timeout: downloadTimeout},
		stopCh:     make(chan struct{}),
		reactMap:   make(map[string]int),
		reactQueue: make([]string, 0, reactMapMaxSize),
	}
	s.AddHandler(b.handleReady)
	s.AddHandler(b.handleMessageCreate)
	s.AddHandler(b.handleInteractionCreate)
	s.AddHandler(b.handleReactionAdd)
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

// Start opens the connection and launches background goroutines.
func (b *Bot) Start() {
	go func() {
		if err := b.Open(); err != nil {
			b.l.Error(fmt.Errorf("discord Open: %w", err))
		}
	}()
	go b.runSyncScheduler()
	go b.runSendScheduler()
}

// Close shuts down the bot and stops all schedulers.
func (b *Bot) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	close(b.stopCh)
	return b.session.Close()
}

// ---------------------------------------------------------------------------
// Slash command registration
// ---------------------------------------------------------------------------

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
}

// handleReady registers slash commands via BulkOverwrite, which atomically
// replaces ALL existing guild commands (old commands are automatically removed).
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

// ---------------------------------------------------------------------------
// Reaction feedback handler
// ---------------------------------------------------------------------------

// handleReactionAdd is called whenever any user adds a reaction to any message.
// If the message is a tracked scheduled-send, positive_rating is incremented.
func (b *Bot) handleReactionAdd(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if r.UserID == s.State.User.ID {
		return // ignore the bot's own reactions
	}
	b.reactMu.RLock()
	albumID, ok := b.reactMap[r.MessageID]
	b.reactMu.RUnlock()
	if !ok {
		return
	}
	ctx := context.Background()
	if err := b.imagesUC.IncrAlbumRating(ctx, albumID); err != nil {
		b.l.Error(fmt.Errorf("handleReactionAdd album=%d: %w", albumID, err))
	}
}

// trackScheduledMsg registers a Discord message as a scheduled-send so that
// future reactions on it update the associated album's positive_rating.
// Evicts the oldest entry when the map reaches reactMapMaxSize.
func (b *Bot) trackScheduledMsg(msgID string, albumID int) {
	b.reactMu.Lock()
	defer b.reactMu.Unlock()
	if len(b.reactQueue) >= reactMapMaxSize {
		oldest := b.reactQueue[0]
		b.reactQueue = b.reactQueue[1:]
		delete(b.reactMap, oldest)
	}
	b.reactMap[msgID] = albumID
	b.reactQueue = append(b.reactQueue, msgID)
}

// ---------------------------------------------------------------------------
// Text command handler  (!command [args])
// ---------------------------------------------------------------------------

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
	}
}

// ---------------------------------------------------------------------------
// Interaction dispatcher
// ---------------------------------------------------------------------------

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
	}
}

// ---------------------------------------------------------------------------
// Slash command handlers
//
// All handlers defer the Discord response immediately (well within the 3 s
// window) then do the actual work — which may include pCloud HTTP calls and
// image downloads — in a goroutine, finally editing the deferred reply.
// Images are sent as file attachments so Discord always renders them inline.
// ---------------------------------------------------------------------------

func (b *Bot) cmdImage(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	}()
}

func (b *Bot) cmdRngImage(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	}()
}

func (b *Bot) cmdRngAlbum(s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.deferInteraction(s, i)
	go func() {
		ctx := context.Background()
		// Fetch poolSize images so fitToLimit has candidates to refill after trimming.
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
	}()
}

func (b *Bot) cmdAlbum(s *discordgo.Session, i *discordgo.InteractionCreate) {
	albumName := i.ApplicationCommandData().Options[0].StringValue()
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
	}()
}

func (b *Bot) cmdFullAlbum(s *discordgo.Session, i *discordgo.InteractionCreate) {
	albumName := i.ApplicationCommandData().Options[0].StringValue()
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
	}()
}

// ---------------------------------------------------------------------------
// Text command handlers  (!command)
// ---------------------------------------------------------------------------

func (b *Bot) msgImage(ctx context.Context, s *discordgo.Session, channelID string) {
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
}

func (b *Bot) msgRngImage(ctx context.Context, s *discordgo.Session, channelID string) {
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
}

func (b *Bot) msgRngAlbum(ctx context.Context, s *discordgo.Session, channelID string) {
	imgs, err := b.imagesUC.GetRandomAlbumImages(ctx, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("msgRngAlbum: %w", err))
		_, _ = s.ChannelMessageSend(channelID, "Failed to get random album.")
		return
	}
	b.sendAlbumToChannel(ctx, s, channelID, albumNameFrom(imgs), imgs)
}

func (b *Bot) msgAlbum(ctx context.Context, s *discordgo.Session, channelID, albumName string) {
	imgs, err := b.imagesUC.GetAlbumImages(ctx, albumName, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("msgAlbum %q: %w", albumName, err))
		_, _ = s.ChannelMessageSend(channelID, fmt.Sprintf("Album %q not found or empty.", albumName))
		return
	}
	b.sendAlbumToChannel(ctx, s, channelID, albumName, imgs)
}

func (b *Bot) msgFullAlbum(ctx context.Context, s *discordgo.Session, channelID, albumName string) {
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
	b.sendFullAlbumToThread(ctx, thread.ID, albumName, cover, hasCover, imgs)

	content := fmt.Sprintf("Full album **%s** — %d images posted in <#%s>.", albumName, total, thread.ID)
	_, _ = s.ChannelMessageEdit(channelID, initMsg.ID, content)
}

// ---------------------------------------------------------------------------
// Shared full-album thread sender
// ---------------------------------------------------------------------------

func (b *Bot) sendFullAlbumToThread(
	ctx context.Context,
	threadID, albumName string,
	cover entity.Image,
	hasCover bool,
	imgs []entity.Image,
) {
	if hasCover {
		cover.IsCover = true
		files, err := b.downloadImages(ctx, []entity.Image{cover})
		if err != nil {
			b.l.Error(fmt.Errorf("sendFullAlbumToThread cover %q: %w", albumName, err))
		} else {
			b.channelSendFiles(b.session, threadID, albumName+" — Cover", files)
		}
	}

	// Process non-cover images in pool-sized batches, fitting each to albumBatchSize.
	for start := 0; start < len(imgs); start += albumPoolSize {
		end := start + albumPoolSize
		if end > len(imgs) {
			end = len(imgs)
		}
		files, err := b.downloadAndFit(ctx, imgs[start:end])
		if err != nil {
			b.l.Error(fmt.Errorf("sendFullAlbumToThread batch %d %q: %w", start, albumName, err))
			continue
		}
		b.channelSendFiles(b.session, threadID, "", files)
	}
}

// ---------------------------------------------------------------------------
// Scheduler goroutines
// ---------------------------------------------------------------------------

func (b *Bot) runSyncScheduler() {
	interval := parseDuration(b.cfg.PCloud.SyncInterval, time.Hour)
	hasCredentials := b.cfg.PCloud.AccessToken != "" || b.cfg.PCloud.Username != ""
	if interval <= 0 || !hasCredentials {
		b.l.Info("pCloud sync disabled (no credentials configured or invalid interval)")
		return
	}
	b.doSync()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.doSync()
		case <-b.stopCh:
			return
		}
	}
}

func (b *Bot) doSync() {
	ctx := context.Background()
	b.l.Info("pCloud sync started")
	if err := b.syncUC.SyncImages(ctx); err != nil {
		b.l.Error(fmt.Errorf("doSync: %w", err))
	} else {
		b.l.Info("pCloud sync completed")
	}
}

func (b *Bot) runSendScheduler() {
	interval := parseDuration(b.cfg.Discord.SendInterval, 6*time.Hour)
	channelID := b.cfg.Discord.SendChannelID
	if interval <= 0 || channelID == "" {
		b.l.Info("scheduled send disabled (no channel ID or invalid interval)")
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.doScheduledSend(channelID)
		case <-b.stopCh:
			return
		}
	}
}

func (b *Bot) doScheduledSend(channelID string) {
	ctx := context.Background()
	imgs, albumID, err := b.imagesUC.GetScheduledAlbumImages(ctx, b.cfg.Discord.SendHistorySize, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend GetScheduledAlbumImages: %w", err))
		return
	}
	msg := b.sendAlbumToChannel(ctx, b.session, channelID, albumNameFrom(imgs), imgs)
	// Track the sent message so any user reaction increments the album's rating.
	if msg != nil {
		b.trackScheduledMsg(msg.ID, albumID)
	}
	// Stamp last_sent_at regardless of whether the Discord upload succeeded,
	// so the same album is not retried immediately on the next tick.
	if err := b.imagesUC.MarkAlbumSent(ctx, albumID); err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend MarkAlbumSent: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Image download helpers
// ---------------------------------------------------------------------------

// downloadPool fetches all images from pCloud into memory as fileEntry values.
// Failed individual downloads are skipped and logged.
func (b *Bot) downloadPool(ctx context.Context, imgs []entity.Image) ([]fileEntry, error) {
	entries := make([]fileEntry, 0, len(imgs))
	for _, img := range imgs {
		u, err := b.imagesUC.ResolveURL(ctx, img)
		if err != nil {
			b.l.Error(fmt.Errorf("downloadPool ResolveURL id=%d: %w", img.ID, err))
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			b.l.Error(fmt.Errorf("downloadPool NewRequest id=%d: %w", img.ID, err))
			continue
		}
		resp, err := b.httpClient.Do(req)
		if err != nil {
			b.l.Error(fmt.Errorf("downloadPool Do id=%d: %w", img.ID, err))
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			b.l.Error(fmt.Errorf("downloadPool ReadAll id=%d: %w", img.ID, err))
			continue
		}
		name := img.URL
		if name == "" {
			name = fmt.Sprintf("image_%d.jpg", img.ID)
		}
		entries = append(entries, fileEntry{
			data:    data,
			name:    name,
			isCover: img.IsCover,
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("downloadPool: all %d images failed to download", len(imgs))
	}
	return entries, nil
}

// downloadImages downloads imgs and returns discordgo.File slice directly.
// Used for single-image commands where pool/size fitting is not needed.
func (b *Bot) downloadImages(ctx context.Context, imgs []entity.Image) ([]*discordgo.File, error) {
	pool, err := b.downloadPool(ctx, imgs)
	if err != nil {
		return nil, err
	}
	return entriesToFiles(pool), nil
}

// downloadAndFit downloads imgs as a pool, then applies fitToLimit to produce
// at most albumBatchSize files that fit within discordMsgLimit.
func (b *Bot) downloadAndFit(ctx context.Context, imgs []entity.Image) ([]*discordgo.File, error) {
	pool, err := b.downloadPool(ctx, imgs)
	if err != nil {
		return nil, err
	}
	selected := fitToLimit(pool, albumBatchSize, discordMsgLimit)
	if len(selected) == 0 {
		return nil, fmt.Errorf("downloadAndFit: no images fit within Discord size limit")
	}
	return entriesToFiles(selected), nil
}

// ---------------------------------------------------------------------------
// Send helpers
// ---------------------------------------------------------------------------

// sendAlbumToChannel downloads imgs with pool fitting and sends to channel.
// Returns the sent Discord message (nil on failure) so callers can track it.
func (b *Bot) sendAlbumToChannel(ctx context.Context, s *discordgo.Session, channelID, caption string, imgs []entity.Image) *discordgo.Message {
	files, err := b.downloadAndFit(ctx, imgs)
	if err != nil {
		b.l.Error(fmt.Errorf("sendAlbumToChannel downloadAndFit: %w", err))
		_, _ = s.ChannelMessageSend(channelID, "Failed to download images.")
		return nil
	}
	return b.channelSendFiles(s, channelID, caption, files)
}

// channelSendFiles sends file attachments to a channel with an optional bold caption.
// Returns the sent message (nil on failure).
func (b *Bot) channelSendFiles(s *discordgo.Session, channelID, caption string, files []*discordgo.File) *discordgo.Message {
	if len(files) == 0 {
		return nil
	}
	payload := &discordgo.MessageSend{Files: files}
	if caption != "" {
		payload.Content = "**" + caption + "**"
	}
	msg, err := s.ChannelMessageSendComplex(channelID, payload)
	if err != nil {
		b.l.Error(fmt.Errorf("channelSendFiles: %w", err))
		return nil
	}
	return msg
}

// ---------------------------------------------------------------------------
// Interaction helpers
// ---------------------------------------------------------------------------

// deferInteraction acknowledges a slash command immediately so Discord doesn't
// show an error. The bot then has up to 15 minutes to edit the response.
func (b *Bot) deferInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		b.l.Error(fmt.Errorf("deferInteraction: %w", err))
	}
}

// editInteractionFiles edits the deferred interaction response with file attachments.
func (b *Bot) editInteractionFiles(s *discordgo.Session, i *discordgo.InteractionCreate, caption string, files []*discordgo.File) {
	edit := &discordgo.WebhookEdit{Files: files}
	if caption != "" {
		c := "**" + caption + "**"
		edit.Content = &c
	}
	if _, err := s.InteractionResponseEdit(i.Interaction, edit); err != nil {
		b.l.Error(fmt.Errorf("editInteractionFiles: %w", err))
	}
}

// editInteractionContent edits the deferred interaction response with plain text.
func (b *Bot) editInteractionContent(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &content}); err != nil {
		b.l.Error(fmt.Errorf("editInteractionContent: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func albumNameFrom(imgs []entity.Image) string {
	if len(imgs) > 0 && imgs[0].AlbumName != "" {
		return imgs[0].AlbumName
	}
	return ""
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
