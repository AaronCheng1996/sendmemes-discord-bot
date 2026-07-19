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

	// videoUploadLimit is a conservative default bot attachment cap for
	// non-boosted guilds. Videos larger than this (or of unknown size) are
	// delivered as a permanent pCloud public link instead of a file attachment.
	videoUploadLimit = 10 * 1024 * 1024

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
// Strategy:
//  1. Shuffle non-cover candidates for random selection order.
//  2. Fill selected with cover + first targetCount−1 shuffled candidates.
//  3. Single loop until one of three conditions is met:
//     – Condition 1: selected == targetCount and total size ≤ maxBytes.
//     – Condition 2: total size ≤ maxBytes but pool exhausted (sends what we have).
//     – Condition 3: pool exhausted with nothing fitting — logs a warning and returns nil.
//     Within the loop: if over limit, remove the largest non-cover then refill with the
//     next shuffled candidate; repeat until within limit or conditions above are met.
func fitToLimit(l logger.Interface, pool []fileEntry, targetCount, maxBytes int) []fileEntry {
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

	// Shuffle for random selection order from the start.
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	// Build initial selection: cover + first (targetCount−1) shuffled candidates.
	selected := make([]fileEntry, 0, targetCount)
	totalBytes := 0
	if cover != nil {
		selected = append(selected, *cover)
		totalBytes += cover.size()
	}
	nextIdx := 0
	for nextIdx < len(candidates) && len(selected) < targetCount {
		selected = append(selected, candidates[nextIdx])
		totalBytes += candidates[nextIdx].size()
		nextIdx++
	}

	// Single loop: trim if over limit, refill from next in shuffled order.
	for {
		if totalBytes <= maxBytes {
			// Condition 1: full and within limit. Condition 2: pool exhausted.
			if len(selected) == targetCount || nextIdx >= len(candidates) {
				break
			}
			// Room for more; add next candidate from shuffled order.
			selected = append(selected, candidates[nextIdx])
			totalBytes += candidates[nextIdx].size()
			nextIdx++
			continue
		}

		// Over limit: remove the largest non-cover image.
		maxIdx, maxSz := -1, 0
		for j, fe := range selected {
			if !fe.isCover && fe.size() > maxSz {
				maxSz = fe.size()
				maxIdx = j
			}
		}
		if maxIdx == -1 {
			// Only cover remains and it alone exceeds the limit — condition 3.
			l.Warn("fitToLimit: cover alone exceeds Discord size limit, skipping message")
			return nil
		}
		totalBytes -= selected[maxIdx].size()
		selected = append(selected[:maxIdx], selected[maxIdx+1:]...)

		// Refill with the next candidate in shuffled order.
		if nextIdx < len(candidates) {
			selected = append(selected, candidates[nextIdx])
			totalBytes += candidates[nextIdx].size()
			nextIdx++
		}
	}

	// Condition 3: all candidates exhausted without a single image fitting.
	if len(selected) == 0 {
		l.Warn("fitToLimit: no images fit within Discord size limit, skipping message")
		return nil
	}

	return selected
}

// chunkOrdered packs pool into sequential chunks WITHOUT reordering or shuffling.
// A new chunk begins when adding the next file would exceed maxCount files or
// maxBytes total bytes. A single file larger than maxBytes on its own cannot fit
// any chunk and is skipped with a warning. Input order is always preserved.
func chunkOrdered(l logger.Interface, pool []fileEntry, maxCount, maxBytes int) [][]fileEntry {
	var chunks [][]fileEntry
	var cur []fileEntry
	curBytes := 0
	for _, fe := range pool {
		if fe.size() > maxBytes {
			l.Warn("chunkOrdered: file %q (%d bytes) exceeds Discord size limit, skipping", fe.name, fe.size())
			continue
		}
		if len(cur) > 0 && (len(cur) >= maxCount || curBytes+fe.size() > maxBytes) {
			chunks = append(chunks, cur)
			cur = nil
			curBytes = 0
		}
		cur = append(cur, fe)
		curBytes += fe.size()
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
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
	cfg           *config.Config
	l             logger.Interface
	uc            usecase.Translation
	imagesUC      usecase.Images
	syncUC        usecase.Sync
	rulesUC       usecase.Rules
	appSettingsUC usecase.AppSettings
	session       *discordgo.Session
	httpClient    *http.Client
	mu            sync.Mutex
	closed        bool
	stopCh        chan struct{}

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
	rulesUC usecase.Rules,
	appSettingsUC usecase.AppSettings,
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
		cfg:           cfg,
		l:             l,
		uc:            uc,
		imagesUC:      imagesUC,
		syncUC:        syncUC,
		rulesUC:       rulesUC,
		appSettingsUC: appSettingsUC,
		session:       s,
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
	go b.runScheduleManager()
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
		return
	}
	b.vlog("reaction feedback: userID=%s emoji=%s albumID=%d messageID=%s",
		r.UserID, r.Emoji.Name, albumID, r.MessageID)
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
// Shared full-album thread sender
// ---------------------------------------------------------------------------

func (b *Bot) sendFullAlbumToThread(
	ctx context.Context,
	threadID, albumName string,
	cover entity.Image,
	hasCover bool,
	imgs []entity.Image,
) {
	totalBatches := (len(imgs) + albumPoolSize - 1) / albumPoolSize
	if hasCover {
		totalBatches++ // cover is batch 0
	}
	batchNum := 0

	if hasCover {
		batchNum++
		b.vlog("full_album %q: sending cover (batch %d/%d)", albumName, batchNum, totalBatches)
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
		batchNum++
		b.vlog("full_album %q: sending batch %d/%d (images %d–%d)", albumName, batchNum, totalBatches, start+1, end)
		files, err := b.downloadAndFit(ctx, imgs[start:end])
		if err != nil {
			b.l.Error(fmt.Errorf("sendFullAlbumToThread batch %d %q: %w", batchNum, albumName, err))
			continue
		}
		b.channelSendFiles(b.session, threadID, "", files)
		b.vlog("full_album %q: batch %d/%d sent (%d files)", albumName, batchNum, totalBatches, len(files))
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
	selected := fitToLimit(b.l, pool, albumBatchSize, discordMsgLimit)
	if len(selected) == 0 {
		return nil, fmt.Errorf("downloadAndFit: no images fit within Discord size limit")
	}
	return entriesToFiles(selected), nil
}

// ---------------------------------------------------------------------------
// Send helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Typed album delivery
// ---------------------------------------------------------------------------

// deliverAlbum sends album to channelID according to album.SendMode and returns
// the root Discord message (nil on failure). captionPrefix is prepended to the
// album name in captions (e.g. "[TEST] "). Delivery formats:
//   - Random/Custom/default: a size-fitted batch of random images (cover-first).
//   - Single: exactly one image.
//   - Order: an ordered comic; first batch only, /full_album for the rest.
//   - Video: one random video as an attachment (small) or public link (large).
func (b *Bot) deliverAlbum(ctx context.Context, channelID string, album entity.Album, captionPrefix string) *discordgo.Message {
	switch album.SendMode {
	case entity.AlbumSendModeSingle:
		return b.deliverSingle(ctx, channelID, album, captionPrefix)
	case entity.AlbumSendModeOrder:
		return b.deliverComic(ctx, channelID, album, captionPrefix)
	case entity.AlbumSendModeVideo:
		return b.deliverVideo(ctx, channelID, album, captionPrefix)
	case entity.AlbumSendModeRandom, entity.AlbumSendModeCustom:
		return b.deliverRandom(ctx, channelID, album, captionPrefix)
	default:
		return b.deliverRandom(ctx, channelID, album, captionPrefix)
	}
}

// deliverRandom sends a size-fitted batch of random images (cover-first).
func (b *Bot) deliverRandom(ctx context.Context, channelID string, album entity.Album, captionPrefix string) *discordgo.Message {
	imgs, err := b.imagesUC.GetAlbumBatch(ctx, album, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum random GetAlbumBatch %q: %w", album.Name, err))
		return nil
	}
	return b.sendAlbumToChannel(ctx, b.session, channelID, captionPrefix+album.Name, imgs)
}

// deliverSingle sends exactly one image from the album (cover when present).
func (b *Bot) deliverSingle(ctx context.Context, channelID string, album entity.Album, captionPrefix string) *discordgo.Message {
	imgs, err := b.imagesUC.GetAlbumBatch(ctx, album, 1)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum single GetAlbumBatch %q: %w", album.Name, err))
		return nil
	}
	files, err := b.downloadImages(ctx, imgs)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum single download %q: %w", album.Name, err))
		return nil
	}
	return b.channelSendFiles(b.session, channelID, captionPrefix+album.Name, files)
}

// deliverComic sends the album as an ordered comic: only the first ordered
// batch (up to albumBatchSize pages within the Discord size limit) is posted to
// the channel. When the album has more pages than fit in that batch, the caption
// points viewers to /full_album (or the full-album button) for the rest; nothing
// else is sent here. Page order is never shuffled.
func (b *Bot) deliverComic(ctx context.Context, channelID string, album entity.Album, captionPrefix string) *discordgo.Message {
	pages, err := b.imagesUC.GetComicPages(ctx, album)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum comic GetComicPages %q: %w", album.Name, err))
		return nil
	}
	pool, err := b.downloadPool(ctx, pages)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum comic downloadPool %q: %w", album.Name, err))
		return nil
	}
	chunks := chunkOrdered(b.l, pool, albumBatchSize, discordMsgLimit)
	if len(chunks) == 0 {
		b.l.Warn("deliverAlbum comic: no pages fit within Discord size limit (album %q)", album.Name)
		return nil
	}

	first := chunks[0]
	totalPages := len(pages)

	caption := fmt.Sprintf("**%s%s** (%d pages)", captionPrefix, album.Name, totalPages)
	if len(first) < totalPages {
		caption = fmt.Sprintf("**%s%s** (showing first %d of %d pages — use /full_album for the rest)",
			captionPrefix, album.Name, len(first), totalPages)
	}
	return b.channelSendFilesContent(channelID, caption, entriesToFiles(first))
}

// deliverVideo posts one random video from the album. Videos within
// videoUploadLimit are uploaded as attachments; larger or unknown-size videos are
// posted as a permanent pCloud public link. Returns nil (sending nothing) when the
// album has no videos.
func (b *Bot) deliverVideo(ctx context.Context, channelID string, album entity.Album, captionPrefix string) *discordgo.Message {
	video, found, err := b.imagesUC.GetRandomVideo(ctx, album.ID)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum video GetRandomVideo %q: %w", album.Name, err))
		return nil
	}
	if !found {
		b.l.Warn("deliverAlbum video: album %q has no videos", album.Name)
		return nil
	}

	caption := captionPrefix + album.Name
	if video.SizeBytes > 0 && video.SizeBytes <= videoUploadLimit {
		files, derr := b.downloadImages(ctx, []entity.Image{video})
		if derr != nil {
			b.l.Error(fmt.Errorf("deliverAlbum video download %q: %w", album.Name, derr))
			return nil
		}
		return b.channelSendFiles(b.session, channelID, caption, files)
	}

	// Over the upload limit or unknown size: fall back to a permanent public link.
	url, rerr := b.imagesUC.ResolvePublicURL(ctx, video)
	if rerr != nil {
		b.l.Error(fmt.Errorf("deliverAlbum video ResolvePublicURL %q: %w", album.Name, rerr))
		return nil
	}
	content := fmt.Sprintf("**%s**\n%s", caption, url)
	msg, serr := b.session.ChannelMessageSend(channelID, content)
	if serr != nil {
		b.l.Error(fmt.Errorf("deliverAlbum video ChannelMessageSend %q: %w", album.Name, serr))
		return nil
	}
	return msg
}

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

// channelSendFilesContent sends file attachments with a raw, already-formatted
// content string. Unlike channelSendFiles it does not wrap the content in bold,
// so callers control the exact markdown. Returns the sent message (nil on failure).
func (b *Bot) channelSendFilesContent(channelID, content string, files []*discordgo.File) *discordgo.Message {
	if len(files) == 0 {
		return nil
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Files:   files,
		Content: content,
	})
	if err != nil {
		b.l.Error(fmt.Errorf("channelSendFilesContent: %w", err))
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
// Verbose logging helper
// ---------------------------------------------------------------------------

// vlog emits an info log only when SENDMEMES_DISCORD_VERBOSE_LOG is enabled.
// Use this for per-request and per-batch operational messages.
func (b *Bot) vlog(format string, args ...interface{}) {
	if b.cfg.Discord.VerboseLog {
		b.l.Info(format, args...)
	}
}

// interactionUser returns a display name for the user who triggered a slash command.
func interactionUser(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.Username
	}
	if i.User != nil {
		return i.User.Username
	}
	return "unknown"
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

// TriggerScheduleNow sends a random album immediately to channelID.
func (b *Bot) TriggerScheduleNow(ctx context.Context, channelID string, historySize int) (entity.ManualScheduleTriggerResult, error) {
	_ = ctx
	ch := strings.TrimSpace(channelID)
	if ch == "" {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("send channel is not configured")
	}
	return b.doScheduledSend(ch, historySize)
}

// SendAlbumTest posts a one-off preview of albumID to channelID.
// It does not call MarkAlbumSent and does not affect anti-repeat scheduling.
func (b *Bot) SendAlbumTest(ctx context.Context, channelID string, albumID int) (entity.ManualScheduleTriggerResult, error) {
	ch := strings.TrimSpace(channelID)
	if ch == "" {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("send channel is not configured")
	}
	album, err := b.imagesUC.GetAlbumByID(ctx, albumID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	msg := b.deliverAlbum(ctx, ch, album, "[TEST] ")
	if msg == nil {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("failed to send test preview (see server logs)")
	}
	return entity.ManualScheduleTriggerResult{
		Triggered: true,
		AlbumID:   album.ID,
		AlbumName: album.Name,
		ChannelID: ch,
		MessageID: msg.ID,
	}, nil
}

// TriggerSyncNow runs a pCloud sync immediately and posts notifications.
func (b *Bot) TriggerSyncNow(ctx context.Context) (entity.SyncReport, error) {
	report, err := b.syncUC.SyncImages(ctx)
	if err != nil {
		return entity.SyncReport{}, err
	}
	b.notifySyncEvents(ctx, report)
	return report, nil
}

// GetDiscordStatus returns current session online status and username.
func (b *Bot) GetDiscordStatus(ctx context.Context) (bool, string) {
	_ = ctx
	if b.session == nil || b.session.State == nil || b.session.State.User == nil {
		return false, ""
	}
	return b.session.DataReady, b.session.State.User.Username
}
