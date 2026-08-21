// Package discord implements Discord bot controller (entry layer).
package discord

import (
	"bytes"
	"context"
	"errors"
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

	// uploadOverheadBytes is held back from Discord's advertised attachment
	// limit. That limit applies to the whole multipart request rather than the
	// file bytes alone, and a message rejected for being slightly over loses
	// every attachment in it, not just the one that pushed it over.
	uploadOverheadBytes = 512 * 1024

	// minUploadBudget keeps a misconfigured DISCORD_UPLOAD_LIMIT_MB from
	// producing a budget nothing can fit in.
	minUploadBudget = 1024 * 1024

	// downloadTimeout is used for both pCloud downloads and Discord uploads.
	// Large albums can have multi-MB images; give plenty of headroom.
	downloadTimeout = 5 * time.Minute

	// reactMapMaxSize is the maximum number of scheduled-send messages tracked
	// for reaction-based feedback.  Oldest entries are evicted when full.
	reactMapMaxSize = 200

	// sendFailureNoticeInterval is the minimum gap between failure notices in
	// one channel. A full-album post is dozens of messages and whatever breaks
	// one usually breaks the rest, so an unthrottled notice would turn a failed
	// post into a second, louder failure.
	sendFailureNoticeInterval = 30 * time.Second
)

// boostTierUploadLimits is the attachment limit a server's boost level grants
// everyone posting in it. Levels below 2 grant nothing beyond the poster's own
// account limit, so they are absent here.
var boostTierUploadLimits = map[discordgo.PremiumTier]int{
	discordgo.PremiumTier2: 50 * 1024 * 1024,
	discordgo.PremiumTier3: 100 * 1024 * 1024,
}

// uploadLimit is the byte budget for one message posted to channelID.
//
// Guessing high is the expensive mistake here: Discord rejects an over-budget
// request outright with 40005, so the whole batch is lost rather than trimmed,
// which looks exactly like the images never existing. The configured floor is
// therefore treated as authoritative unless the channel's guild is known to
// grant more.
func (b *Bot) uploadLimit(channelID string) int {
	limit := b.cfg.Discord.UploadLimitMB * 1024 * 1024
	if boost, ok := b.guildBoostLimit(channelID); ok && boost > limit {
		limit = boost
	}
	if budget := limit - uploadOverheadBytes; budget >= minUploadBudget {
		return budget
	}
	return minUploadBudget
}

// guildBoostLimit resolves channelID's guild boost allowance from the gateway
// state cache (populated by IntentsGuilds). Not found -- a DM, or a thread the
// cache has not seen -- leaves the configured floor standing, which is the safe
// direction to be wrong in.
func (b *Bot) guildBoostLimit(channelID string) (int, bool) {
	if b.session == nil || b.session.State == nil {
		return 0, false
	}
	ch, err := b.session.State.Channel(channelID)
	if err != nil || ch.GuildID == "" {
		return 0, false
	}
	guild, err := b.session.State.Guild(ch.GuildID)
	if err != nil {
		return 0, false
	}
	limit, ok := boostTierUploadLimits[guild.PremiumTier]
	return limit, ok
}

// fileEntry is an already-downloaded image file, kept in memory so that
// fitToLimit can inspect sizes and reassemble the final Discord file list
// without extra network round-trips.
type fileEntry struct {
	data    []byte
	name    string
	isCover bool
}

func (f fileEntry) size() int { return len(f.data) }

// fitToLimit picks one message worth of files out of pool and discards the rest.
// It is the sampling half of the pair — chunkOrdered is the packing half, for
// callers that must post everything.
//
// The album cover leads the selection when there is one, then shuffled
// candidates fill up to targetCount. While the selection is over maxBytes the
// largest non-cover file is evicted and the next candidate takes its place, so
// a batch of big images degrades into fewer images rather than none.
//
// Returns nil only when not one file fits on its own.
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

	// A cover bigger than the budget can never be sent, and pinning it drags the
	// message down with it: every candidate gets evicted below trying to make
	// room that does not exist, and the message ends up empty even though
	// ordinary images would have fitted. Drop it before selecting anything.
	if cover != nil && cover.size() > maxBytes {
		l.Warn("fitToLimit: cover %q (%d bytes) is over the %d-byte budget, selecting without it", cover.name, cover.size(), maxBytes)
		cover = nil
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
			// Unreachable once an oversized cover is dropped up front, since a
			// cover that fits cannot be over the limit on its own. Kept as a
			// guard rather than a panic.
			l.Warn("fitToLimit: nothing left to evict and still over budget, skipping message")
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
// maxBytes total bytes. Input order is always preserved.
//
// A single file larger than maxBytes cannot fit any chunk and is returned in
// oversized instead. Callers that post to a channel are expected to tell the
// reader which files those were: dropping them silently looks identical to the
// album simply not containing them.
func chunkOrdered(l logger.Interface, pool []fileEntry, maxCount, maxBytes int) (chunks [][]fileEntry, oversized []fileEntry) {
	var cur []fileEntry
	curBytes := 0
	for _, fe := range pool {
		if fe.size() > maxBytes {
			l.Warn("chunkOrdered: file %q (%d bytes) exceeds Discord size limit, skipping", fe.name, fe.size())
			oversized = append(oversized, fe)
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
	return chunks, oversized
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

	// noticeMu guards lastNotice, the per-channel timestamp that throttles
	// send-failure notices.
	noticeMu   sync.Mutex
	lastNotice map[string]time.Time
}

// NewBot creates a Discord bot that delegates to use cases.
func NewBot(
	cfg *config.Config,
	l logger.Interface,
	imagesUC usecase.Images,
	syncUC usecase.Sync,
	rulesUC usecase.Rules,
	appSettingsUC usecase.AppSettings,
) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("discord NewSession: %w", err)
	}
	// IntentsGuilds populates the state cache with guilds and channels, which
	// is how uploadLimit learns a server's boost tier. It is not a privileged
	// intent, so it needs no approval in the developer portal.
	s.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsGuildMessageReactions

	// discordgo defaults to 20 s — far too short for uploading many large images.
	s.Client = &http.Client{Timeout: downloadTimeout}

	b := &Bot{
		cfg:           cfg,
		l:             l,
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
		lastNotice: make(map[string]time.Time),
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

// fullAlbumPost is everything the three /full_album entry points look up before
// any of them can open a thread. They differ in where the thread hangs and how
// they reply, not in what they need loaded.
type fullAlbumPost struct {
	Album    entity.Album
	Cover    entity.Image
	HasCover bool
	Images   []entity.Image // non-cover, ordered by id
}

// Total counts the cover alongside the album's other images.
func (p fullAlbumPost) Total() int {
	if p.HasCover {
		return len(p.Images) + 1
	}
	return len(p.Images)
}

// loadFullAlbum reads an album by name for a full-album post, treating an empty
// album as a failure since there is nothing to open a thread for. The returned
// error is already worded for a channel.
func (b *Bot) loadFullAlbum(ctx context.Context, albumName string) (fullAlbumPost, error) {
	cover, hasCover, err := b.imagesUC.GetAlbumCover(ctx, albumName)
	if err != nil {
		b.l.Error(fmt.Errorf("loadFullAlbum GetAlbumCover %q: %w", albumName, err))
		return fullAlbumPost{}, fmt.Errorf("album **%s** not found", albumName)
	}
	imgs, err := b.imagesUC.GetFullAlbum(ctx, albumName)
	if err != nil {
		b.l.Error(fmt.Errorf("loadFullAlbum GetFullAlbum %q: %w", albumName, err))
		return fullAlbumPost{}, fmt.Errorf("album **%s** not found", albumName)
	}
	post := fullAlbumPost{
		Album:    albumRefFrom(albumName, cover, hasCover, imgs),
		Cover:    cover,
		HasCover: hasCover,
		Images:   imgs,
	}
	if post.Total() == 0 {
		return fullAlbumPost{}, fmt.Errorf("album **%s** is empty", albumName)
	}
	return post, nil
}

// startAlbumThread opens the thread a full-album post goes into, hanging it off
// whichever message the request produced.
func (b *Bot) startAlbumThread(channelID, messageID, albumName string) (*discordgo.Channel, error) {
	return b.session.MessageThreadStartComplex(channelID, messageID, &discordgo.ThreadStart{
		Name:                fmt.Sprintf("Full album: %s", albumName),
		AutoArchiveDuration: 60,
		Type:                discordgo.ChannelTypeGuildPublicThread,
	})
}

// albumRefFrom builds the minimal album identity a full-album post needs — the
// id for the continue button, the name for captions — out of rows already
// fetched, so the name-only entry points need no extra lookup.
func albumRefFrom(name string, cover entity.Image, hasCover bool, imgs []entity.Image) entity.Album {
	album := entity.Album{Name: name}
	switch {
	case len(imgs) > 0:
		album.ID = imgs[0].AlbumID
	case hasCover:
		album.ID = cover.AlbumID
	}
	return album
}

// fullAlbumPageSizeFallback is used when FULL_ALBUM_PAGE_SIZE is misconfigured
// (below 1), which would otherwise make a page hold nothing and the continue
// button never advance.
const fullAlbumPageSizeFallback = 100

// fullAlbumPaging resolves the configured page threshold and size.
// paged is false when the album is small enough to post in one go.
func (b *Bot) fullAlbumPaging(total int) (pageSize int, paged bool) {
	threshold := b.cfg.Discord.FullAlbumPageThreshold
	if threshold <= 0 || total <= threshold {
		return total, false
	}
	pageSize = b.cfg.Discord.FullAlbumPageSize
	if pageSize < 1 {
		b.l.Warn("full_album: FULL_ALBUM_PAGE_SIZE=%d is invalid, using %d", pageSize, fullAlbumPageSizeFallback)
		pageSize = fullAlbumPageSizeFallback
	}
	return pageSize, true
}

// sendFullAlbumPage posts album images into channelID starting at offset (an
// index into imgs, which holds the album's non-cover images in a stable order).
//
// Every image that downloads and fits within Discord's per-message limit is
// posted — the pool is packed sequentially into as many messages as it takes,
// rather than selecting a subset of it. Only a file too large to share a message
// with nothing else is skipped, and those are named in a trailing notice.
//
// Albums above the configured threshold stop after one page and close with a
// button that resumes here at the next offset, so a thousand-image album does
// not dump a thousand images into one thread unasked.
//
// Returns how many images this page posted and how many the album still has
// waiting, so the caller can report the page rather than the whole album.
func (b *Bot) sendFullAlbumPage(
	ctx context.Context,
	channelID string,
	album entity.Album,
	cover entity.Image,
	hasCover bool,
	imgs []entity.Image,
	offset int,
) (sent, remaining int) {
	if offset < 0 || offset > len(imgs) {
		b.l.Error(fmt.Errorf("sendFullAlbumPage %q: offset %d out of range (have %d images)", album.Name, offset, len(imgs)))
		return 0, 0
	}

	pageSize, paged := b.fullAlbumPaging(len(imgs))
	// Resolved once per page: every message here goes to the same channel, so
	// the budget cannot change mid-page.
	budget := b.uploadLimit(channelID)
	end := offset + pageSize
	if end > len(imgs) {
		end = len(imgs)
	}

	var oversized []fileEntry

	// The cover leads the album, so it belongs to the first page only. It goes
	// through the same budget check as everything else: it used to be handed
	// straight to Discord, so an oversized cover produced a 40005 rejection
	// rather than a line in the skipped-files notice.
	if hasCover && offset == 0 {
		b.vlog("full_album %q: sending cover", album.Name)
		cover.IsCover = true
		pool, err := b.downloadPool(ctx, []entity.Image{cover})
		switch {
		case err != nil:
			b.l.Error(fmt.Errorf("sendFullAlbumPage cover %q: %w", album.Name, err))
		case pool[0].size() > budget:
			oversized = append(oversized, pool[0])
		default:
			b.channelSendFiles(channelID, album.Name+" — Cover", albumLabel(album), pool)
		}
	}

	// Download in pool-sized windows to bound memory, but post every window in
	// full: chunkOrdered splits it across as many messages as needed.
	for start := offset; start < end; start += albumPoolSize {
		stop := start + albumPoolSize
		if stop > end {
			stop = end
		}
		b.vlog("full_album %q: downloading images %d–%d of %d", album.Name, start+1, stop, len(imgs))
		pool, err := b.downloadPool(ctx, imgs[start:stop])
		if err != nil {
			b.l.Error(fmt.Errorf("sendFullAlbumPage window %d–%d %q: %w", start+1, stop, album.Name, err))
			continue
		}
		chunks, tooBig := chunkOrdered(b.l, pool, albumBatchSize, budget)
		oversized = append(oversized, tooBig...)
		for _, chunk := range chunks {
			b.channelSendFiles(channelID, "", albumLabel(album), chunk)
			sent += len(chunk)
		}
	}
	b.vlog("full_album %q: posted %d image(s) from offset %d (%d skipped as oversized)",
		album.Name, sent, offset, len(oversized))

	if len(oversized) > 0 {
		if _, err := b.session.ChannelMessageSend(channelID, oversizedNotice(oversized, budget)); err != nil {
			b.l.Error(fmt.Errorf("sendFullAlbumPage oversized notice %q: %w", album.Name, err))
		}
	}

	remaining = len(imgs) - end
	if paged && remaining > 0 {
		content := fmt.Sprintf("Posted %d of %d images. %d left.", end, len(imgs), remaining)
		if b.channelSendPlain(channelID, content, albumLabel(album), nil, fullAlbumMoreButtonRow(album.ID, end, remaining)) == nil {
			b.l.Error(fmt.Errorf("sendFullAlbumPage %q: failed to post the continue button at offset %d", album.Name, end))
		}
	}
	return sent, remaining
}

// fullAlbumSummary words the "done" reply for a full-album post. A paged album
// has only posted its first page, so reporting the album's total would claim
// more than actually went out.
func fullAlbumSummary(albumName, threadID string, sent, remaining int) string {
	if remaining > 0 {
		return fmt.Sprintf("Full album **%s** — %d images posted in <#%s>, %d more behind the button there.",
			albumName, sent, threadID, remaining)
	}
	return fmt.Sprintf("Full album **%s** — %d images posted in <#%s>.", albumName, sent, threadID)
}

// oversizedNotice renders the "these files were skipped" message. Sizes are
// included because the fix is on the reader's side — they have to go shrink or
// fetch the file themselves.
func oversizedNotice(oversized []fileEntry, budget int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "⚠️ Skipped %d file(s) too large for Discord (limit %s):",
		len(oversized), humanBytes(budget))
	for _, fe := range oversized {
		fmt.Fprintf(&sb, "\n• `%s` — %s", fe.name, humanBytes(fe.size()))
	}
	return sb.String()
}

// humanBytes renders a byte count as MB/KB for a Discord message.
func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
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

// errNothingFits reports that every downloaded image was individually larger
// than the message budget. It is a different problem from a download that
// failed, and the operator fixes it differently, so the two are not merged.
var errNothingFits = errors.New("no image fits within the upload budget")

// downloadAndFit downloads imgs as a pool, then applies fitToLimit to produce
// at most albumBatchSize files that fit channelID's upload budget.
func (b *Bot) downloadAndFit(ctx context.Context, channelID string, imgs []entity.Image) ([]fileEntry, error) {
	return b.downloadAndFitN(ctx, channelID, imgs, albumBatchSize)
}

// downloadAndFitN is downloadAndFit with a caller-chosen target count, used by
// delivery paths whose per-album send config overrides the batch size.
func (b *Bot) downloadAndFitN(ctx context.Context, channelID string, imgs []entity.Image, targetCount int) ([]fileEntry, error) {
	pool, err := b.downloadPool(ctx, imgs)
	if err != nil {
		return nil, err
	}
	selected := fitToLimit(b.l, pool, targetCount, b.uploadLimit(channelID))
	if len(selected) == 0 {
		return nil, errNothingFits
	}
	return selected, nil
}

// ---------------------------------------------------------------------------
// Typed album delivery
// ---------------------------------------------------------------------------

// deliverAlbum sends album to channelID according to album.SendMode and returns
// the root Discord message (nil on failure). sc describes the delivery itself
// (test preview, originating rule) and supplies the context-dependent caption
// placeholders; base is the app defaults merged with the delivery rule, on top
// of which the album's own style wins. album.SendConfigJSON is parsed once here into an
// AlbumSendConfig and threaded into every delivery path: Custom mode honors
// all of its fields (BatchSize/IncludeCover/Ordered/Caption/NSFW), while the
// other modes honor the fields that are meaningful for their fixed shape
// (BatchSize where the mode has a variable-size batch, Caption and NSFW
// always). Delivery formats:
//   - Random/default: a size-fitted batch of random images (cover-first).
//   - Custom: batch built per the album's send config (see deliverCustom).
//   - Single: exactly one random image (no cover pinning — see deliverSingle).
//   - Order: an ordered comic; first batch only, /full_album for the rest.
//   - Video: one random video as an attachment (small) or public link (large).
func (b *Bot) deliverAlbum(ctx context.Context, channelID string, album entity.Album, sc sendContext, base entity.MessageStyle) *discordgo.Message {
	cfg, err := entity.ParseAlbumSendConfig(album.SendConfigJSON)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum %q: invalid send_config_json, using defaults: %w", album.Name, err))
		cfg = entity.AlbumSendConfig{}
	}
	// The album is the top layer: base already carries app defaults merged with
	// the delivery rule, so per-field overrides here win.
	style := entity.MergeMessageStyle(base, cfg.Style())
	switch album.SendMode {
	case entity.AlbumSendModeSingle:
		return b.deliverSingle(ctx, channelID, album, sc, style, cfg)
	case entity.AlbumSendModeOrder:
		return b.deliverComic(ctx, channelID, album, sc, style, cfg)
	case entity.AlbumSendModeVideo:
		return b.deliverVideo(ctx, channelID, album, sc, style, cfg)
	case entity.AlbumSendModeCustom:
		return b.deliverCustom(ctx, channelID, album, sc, style, cfg)
	default:
		return b.deliverRandom(ctx, channelID, album, sc, style, cfg)
	}
}

// appStyle loads the app-wide message defaults, falling back to an empty layer
// (built-in defaults) when the settings cannot be read.
func (b *Bot) appStyle(ctx context.Context) entity.MessageStyle {
	settings, err := b.appSettingsUC.Get(ctx)
	if err != nil {
		b.l.Error(fmt.Errorf("appStyle: %w", err))
		return entity.MessageStyle{}
	}
	return settings.Style()
}

// baseStyleForChannel resolves the app defaults merged with the scheduled rule
// that targets channelID, if any. Test sends and manual triggers use it so the
// preview matches what the real scheduled post will look like — without it a
// rule's title/body/embed settings would be silently ignored.
func (b *Bot) baseStyleForChannel(ctx context.Context, channelID string) (entity.MessageStyle, string) {
	style := b.appStyle(ctx)
	rules, err := b.rulesUC.ListActiveByTrigger(ctx, entity.TriggerScheduled)
	if err != nil {
		b.l.Error(fmt.Errorf("baseStyleForChannel: %w", err))
		return style, ""
	}
	for _, rule := range rules {
		if rule.ChannelID == channelID {
			return entity.MergeMessageStyle(style, rule.Style()), rule.Name
		}
	}
	return style, ""
}

// albumCounts reads how much media the album holds, for the {album_*}
// placeholders. A failure is not worth aborting a send over: the counts come
// back unknown, which leaves those placeholders unexpanded.
func (b *Bot) albumCounts(ctx context.Context, albumID int) albumCounts {
	images, videos, err := b.imagesUC.CountAlbumMedia(ctx, albumID)
	if err != nil {
		b.l.Error(fmt.Errorf("albumCounts %d: %w", albumID, err))
		return albumCounts{}
	}
	return albumCounts{Images: images, Videos: videos, Known: true}
}

// albumMessage resolves style for one album delivery. defaultTotal is what the
// built-in "(showing X of Y)" caption compares shown against — the album's
// images for the image modes, its videos for Video mode.
func albumMessage(style entity.MessageStyle, album entity.Album, shown, defaultTotal int, counts albumCounts, sc sendContext) renderedMessage {
	tokens := albumTokens(album, shown, counts, sc)
	return renderMessage(style, tokens, defaultCaption(album, shown, defaultTotal), sc.Test)
}

// firstEmbeddableName names the attachment an embed should render large.
//
// Only a still image works there — pointing embed.Image at a video leaves an
// empty frame — so it skips past any video in the batch, and returns "" when
// the batch is all video. The video still uploads and Discord gives it its own
// inline player below the embed.
func firstEmbeddableName(files []fileEntry) string {
	for _, f := range files {
		if kind, ok := entity.KindOfExtension(f.name); ok && kind == entity.MediaKindImage {
			return f.name
		}
	}
	return ""
}

// batchSizeOrDefault returns cfg.BatchSize when positive, else fall.
func batchSizeOrDefault(cfg entity.AlbumSendConfig, fall int) int {
	if cfg.BatchSize > 0 {
		return cfg.BatchSize
	}
	return fall
}

// spoilerPrefix marks an attachment as a spoiler in Discord (auto-blurred until
// clicked) when applied as a filename prefix.
const spoilerPrefix = "SPOILER_"

// applySpoiler prefixes every file's name with spoilerPrefix when nsfw is set,
// so config.NSFW albums post their attachments blurred behind a click-to-reveal.
// It renames in place.
func applySpoiler(files []fileEntry, nsfw bool) {
	if !nsfw {
		return
	}
	for i := range files {
		if !strings.HasPrefix(files[i].name, spoilerPrefix) {
			files[i].name = spoilerPrefix + files[i].name
		}
	}
}

// excludeCover returns imgs without any image flagged as the album cover,
// used by deliverCustom when send-config IncludeCover is explicitly false.
func excludeCover(imgs []entity.Image) []entity.Image {
	out := make([]entity.Image, 0, len(imgs))
	for _, img := range imgs {
		if !img.IsCover {
			out = append(out, img)
		}
	}
	return out
}

// resolveThumbURL returns a public thumbnail URL for the first of imgs, used
// as the embed's small corner thumbnail. Thumbnails are a nice-to-have, not
// required for delivery to succeed, so any failure (or an empty imgs) just
// means no thumbnail rather than an aborted send.
func (b *Bot) resolveThumbURL(ctx context.Context, imgs []entity.Image) string {
	for _, img := range imgs {
		// A video has no still to thumbnail, so keep looking.
		if img.Kind == entity.MediaKindVideo {
			continue
		}
		if url, err := b.imagesUC.ResolvePreviewURL(ctx, img); err == nil {
			return url
		}
		return ""
	}
	return ""
}

// deliverRandom sends a size-fitted batch of random images (cover-first) with a
// "Full album" button that expands the whole album into a thread on demand.
// cfg.BatchSize overrides the batch/pool size and cfg.Caption/cfg.NSFW apply as
// usual (see effectiveCaptionTemplate/applySpoiler).
func (b *Bot) deliverRandom(ctx context.Context, channelID string, album entity.Album, sc sendContext, style entity.MessageStyle, cfg entity.AlbumSendConfig) *discordgo.Message {
	batchSize := batchSizeOrDefault(cfg, albumBatchSize)
	imgs, err := b.imagesUC.GetAlbumBatch(ctx, album, batchSize*2)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum random GetAlbumBatch %q: %w", album.Name, err))
		return nil
	}
	files, err := b.downloadAndFitN(ctx, channelID, imgs, batchSize)
	if err != nil {
		b.reportDeliveryFailure(channelID, album, err)
		return nil
	}
	applySpoiler(files, cfg.NSFW)
	counts := b.albumCounts(ctx, album.ID)
	msg := albumMessage(style, album, len(files), counts.totalOr(len(imgs)), counts, sc)
	return b.sendStyled(channelID, album, msg, b.resolveThumbURL(ctx, imgs), firstEmbeddableName(files), files, fullAlbumButtonRow(album.ID))
}

// deliverSingle sends exactly one *random* image from the album — cover
// pinning is a batch-mode concept (so the cover always leads a multi-image
// post); Single has no "rest of the batch" for the cover to lead, so it picks
// uniformly at random like everything else instead of always being the cover.
// The image count is fixed by definition, so cfg.BatchSize does not apply;
// cfg.Caption/cfg.NSFW still do.
func (b *Bot) deliverSingle(ctx context.Context, channelID string, album entity.Album, sc sendContext, style entity.MessageStyle, cfg entity.AlbumSendConfig) *discordgo.Message {
	imgs, err := b.imagesUC.GetRandomFromAlbum(ctx, album.ID, 1)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum single GetRandomFromAlbum %q: %w", album.Name, err))
		return nil
	}
	files, err := b.downloadPool(ctx, imgs)
	if err != nil {
		b.reportDeliveryFailure(channelID, album, err)
		return nil
	}
	applySpoiler(files, cfg.NSFW)
	counts := b.albumCounts(ctx, album.ID)
	msg := albumMessage(style, album, len(files), counts.totalOr(len(imgs)), counts, sc)
	return b.sendStyled(channelID, album, msg, b.resolveThumbURL(ctx, imgs), firstEmbeddableName(files), files, nil)
}

// deliverComic sends the album as an ordered comic: only the first ordered
// batch (up to albumBatchSize pages within the Discord size limit) is posted to
// the channel. When the album has more pages than fit in that batch, the caption
// points viewers to /full_album (or the full-album button) for the rest; nothing
// else is sent here. Page order is never shuffled.
func (b *Bot) deliverComic(ctx context.Context, channelID string, album entity.Album, sc sendContext, style entity.MessageStyle, cfg entity.AlbumSendConfig) *discordgo.Message {
	batchSize := batchSizeOrDefault(cfg, albumBatchSize)
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
	chunks, _ := chunkOrdered(b.l, pool, batchSize, b.uploadLimit(channelID))
	if len(chunks) == 0 {
		b.l.Warn("deliverAlbum comic: no pages fit within Discord size limit (album %q)", album.Name)
		return nil
	}

	first := chunks[0]
	totalPages := len(pages)
	files := first
	applySpoiler(files, cfg.NSFW)

	counts := b.albumCounts(ctx, album.ID)
	msg := albumMessage(style, album, len(first), counts.totalOr(totalPages), counts, sc)
	if len(first) < totalPages {
		msg.Body += "\nUse /full_album (or the button on a random post) for the rest."
	}
	return b.sendStyled(channelID, album, msg, b.resolveThumbURL(ctx, pages), firstEmbeddableName(files), files, nil)
}

// deliverVideo posts one random video from the album. Videos within the
// channel's upload budget are uploaded as attachments; larger or unknown-size
// videos are posted as a permanent pCloud public link. Returns nil (sending nothing) when the
// album has no videos.
func (b *Bot) deliverVideo(ctx context.Context, channelID string, album entity.Album, sc sendContext, style entity.MessageStyle, cfg entity.AlbumSendConfig) *discordgo.Message {
	video, found, err := b.imagesUC.GetRandomVideo(ctx, album.ID)
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum video GetRandomVideo %q: %w", album.Name, err))
		return nil
	}
	if !found {
		b.l.Warn("deliverAlbum video: album %q has no videos", album.Name)
		return nil
	}

	counts := b.albumCounts(ctx, album.ID)
	msg := albumMessage(style, album, 1, counts.videosOr(1), counts, sc)
	// Video attachments don't render through embed.Image (Discord embeds only
	// preview static images), so the video is just a sibling file attachment.
	if video.SizeBytes > 0 && video.SizeBytes <= int64(b.uploadLimit(channelID)) {
		files, derr := b.downloadPool(ctx, []entity.Image{video})
		if derr != nil {
			b.l.Error(fmt.Errorf("deliverAlbum video download %q: %w", album.Name, derr))
			return nil
		}
		applySpoiler(files, cfg.NSFW)
		return b.sendStyled(channelID, album, msg, "", "", files, nil)
	}

	// Over the upload limit or unknown size: fall back to a permanent public link.
	url, rerr := b.imagesUC.ResolvePublicURL(ctx, video)
	if rerr != nil {
		b.l.Error(fmt.Errorf("deliverAlbum video ResolvePublicURL %q: %w", album.Name, rerr))
		return nil
	}
	msg.Body = strings.TrimSpace(msg.Body + "\n" + url)
	return b.sendStyled(channelID, album, msg, "", "", nil, nil)
}

// deliverCustom builds a batch fully driven by the album's send config: unlike
// the other modes (which have a fixed shape), Custom mode reads BatchSize,
// IncludeCover, Ordered, Caption, and NSFW to decide exactly what goes out.
// Ordered=true walks the album in natural filename order (like Order mode);
// otherwise images are picked cover-first + random (like Random mode).
// IncludeCover=false drops the cover from either listing before batching.
func (b *Bot) deliverCustom(ctx context.Context, channelID string, album entity.Album, sc sendContext, style entity.MessageStyle, cfg entity.AlbumSendConfig) *discordgo.Message {
	batchSize := batchSizeOrDefault(cfg, albumBatchSize)

	var imgs []entity.Image
	var err error
	if cfg.Ordered {
		imgs, err = b.imagesUC.GetComicPages(ctx, album)
	} else {
		imgs, err = b.imagesUC.GetAlbumBatch(ctx, album, batchSize*2)
	}
	if err != nil {
		b.l.Error(fmt.Errorf("deliverAlbum custom fetch %q: %w", album.Name, err))
		return nil
	}
	if cfg.IncludeCover != nil && !*cfg.IncludeCover {
		imgs = excludeCover(imgs)
	}
	if len(imgs) == 0 {
		b.l.Warn("deliverAlbum custom: album %q has no images to send after config filtering", album.Name)
		return nil
	}

	var files []fileEntry
	var sent, total int
	if cfg.Ordered {
		pool, derr := b.downloadPool(ctx, imgs)
		if derr != nil {
			b.l.Error(fmt.Errorf("deliverAlbum custom downloadPool %q: %w", album.Name, derr))
			return nil
		}
		chunks, _ := chunkOrdered(b.l, pool, batchSize, b.uploadLimit(channelID))
		if len(chunks) == 0 {
			b.l.Warn("deliverAlbum custom: no images fit within Discord size limit (album %q)", album.Name)
			return nil
		}
		files = chunks[0]
		sent, total = len(chunks[0]), len(imgs)
	} else {
		files, err = b.downloadAndFitN(ctx, channelID, imgs, batchSize)
		if err != nil {
			b.reportDeliveryFailure(channelID, album, err)
			return nil
		}
		sent, total = len(files), len(imgs)
	}
	applySpoiler(files, cfg.NSFW)

	counts := b.albumCounts(ctx, album.ID)
	msg := albumMessage(style, album, sent, counts.totalOr(total), counts, sc)
	return b.sendStyled(channelID, album, msg, b.resolveThumbURL(ctx, imgs), firstEmbeddableName(files), files, fullAlbumButtonRow(album.ID))
}

// sendAlbumToChannel downloads imgs with pool fitting and sends to channel.
// Returns the sent Discord message (nil on failure) so callers can track it.
func (b *Bot) sendAlbumToChannel(ctx context.Context, s *discordgo.Session, channelID, caption string, imgs []entity.Image) {
	files, err := b.downloadAndFit(ctx, channelID, imgs)
	if err != nil {
		b.l.Error(fmt.Errorf("sendAlbumToChannel downloadAndFit: %w", err))
		_, _ = s.ChannelMessageSend(channelID, "⚠️ Could not post these images (see the server logs).")
		return
	}
	b.channelSendFiles(channelID, caption, caption, files)
}

// restErrorCode returns Discord's numeric error code from err, or 0 when err
// did not come from the Discord API.
func restErrorCode(err error) int {
	var restErr *discordgo.RESTError
	if errors.As(err, &restErr) && restErr.Message != nil {
		return restErr.Message.Code
	}
	return 0
}

// sendFailureNotice words a failed send for the channel it failed in. 40005
// gets its own text because it is both the most common failure and the only one
// the operator can act on directly: the configured budget is above what this
// server actually accepts.
func sendFailureNotice(what string, files []fileEntry, err error, budget int) string {
	subject := "This post"
	if what != "" {
		subject = what
	}
	if restErrorCode(err) == discordgo.ErrCodeRequestEntityTooLarge {
		// Reaching here means the backoff already shed the batch down to one
		// file and Discord still refused it, so naming that file is the useful
		// thing to say.
		if len(files) == 1 {
			return fmt.Sprintf(
				"⚠️ %s — Discord refused `%s` (%s) on its own. The bot is allowing %s per message, "+
					"which is above what this server accepts: lower `DISCORD_UPLOAD_LIMIT_MB`.",
				subject, files[0].name, humanBytes(files[0].size()), humanBytes(budget))
		}
		return fmt.Sprintf(
			"⚠️ %s — Discord rejected %d attachment(s) as too large. The bot is allowing %s per message; "+
				"lower `DISCORD_UPLOAD_LIMIT_MB`.",
			subject, len(files), humanBytes(budget))
	}
	return fmt.Sprintf("⚠️ %s — could not be posted (see the server logs for details).", subject)
}

// albumLabel identifies an album in a channel message. The id is included
// because album names are not unique enough to search the dashboard by.
func albumLabel(album entity.Album) string {
	return fmt.Sprintf("**%s** (album #%d)", album.Name, album.ID)
}

// reportDeliveryFailure explains in the channel why an album produced no post.
// This path used to say "Failed to download images." for both causes, which was
// wrong for the commoner one — the images downloaded fine, they just did not
// fit — and named neither the album nor the size involved.
func (b *Bot) reportDeliveryFailure(channelID string, album entity.Album, err error) {
	b.l.Error(fmt.Errorf("deliver %q: %w", album.Name, err))
	var text string
	if errors.Is(err, errNothingFits) {
		text = fmt.Sprintf(
			"⚠️ %s — every image in it is larger than the %s this channel allows, so nothing could be posted.",
			albumLabel(album), humanBytes(b.uploadLimit(channelID)))
	} else {
		text = fmt.Sprintf("⚠️ %s — the images could not be downloaded (see the server logs).", albumLabel(album))
	}
	if _, sendErr := b.session.ChannelMessageSend(channelID, text); sendErr != nil {
		b.l.Error(fmt.Errorf("reportDeliveryFailure %q: %w", album.Name, sendErr))
	}
}

// noteSendFailure logs a failed send and says so in the channel, at most once
// per sendFailureNoticeInterval there.
//
// Logging alone is not enough: a send that fails quietly is indistinguishable
// from an album that had nothing to post, which is how an over-budget batch
// went unnoticed for a year.
func (b *Bot) noteSendFailure(channelID, what string, files []fileEntry, err error) {
	b.l.Error(fmt.Errorf("send to %s (%s): %w", channelID, what, err))
	if !b.claimFailureNotice(channelID) {
		return
	}
	notice := sendFailureNotice(what, files, err, b.uploadLimit(channelID))
	if _, sendErr := b.session.ChannelMessageSend(channelID, notice); sendErr != nil {
		// Nothing further to try: a channel that refuses a plain text message
		// will not accept an explanation of why it refused the last one.
		b.l.Error(fmt.Errorf("could not post the failure notice to %s: %w", channelID, sendErr))
	}
}

// claimFailureNotice reports whether channelID is due another failure notice,
// recording the attempt when it is.
func (b *Bot) claimFailureNotice(channelID string) bool {
	b.noticeMu.Lock()
	defer b.noticeMu.Unlock()
	if last, ok := b.lastNotice[channelID]; ok && time.Since(last) < sendFailureNoticeInterval {
		return false
	}
	if b.lastNotice == nil {
		b.lastNotice = make(map[string]time.Time)
	}
	b.lastNotice[channelID] = time.Now()
	return true
}

// sendWithBackoff posts payload, shedding half its attachments and retrying
// whenever Discord rejects the request as too large.
//
// The configured budget is a guess at a number Discord never states outright:
// account tier, boost level and per-server policy all move it, and it can
// change under a running bot. A 40005 is the only authoritative reading of the
// real limit, so a rejection drops half the batch and tries again rather than
// throwing the whole post away. Attachments are rebuilt from their entries on
// every attempt because the failed request consumed the previous readers.
//
// what names the subject for the failure notice; empty is allowed.
func (b *Bot) sendWithBackoff(channelID, what string, entries []fileEntry, payload *discordgo.MessageSend) *discordgo.Message {
	for {
		payload.Files = entriesToFiles(entries)
		msg, err := b.session.ChannelMessageSendComplex(channelID, payload)
		if err == nil {
			return msg
		}
		// Keep the first attachment: an embed's attachment:// image points at it.
		if restErrorCode(err) != discordgo.ErrCodeRequestEntityTooLarge || len(entries) <= 1 {
			b.noteSendFailure(channelID, what, entries, err)
			return nil
		}
		entries = entries[:len(entries)/2]
		b.l.Warn("channel %s rejected the message as too large, retrying with %d attachment(s)", channelID, len(entries))
	}
}

// channelSendFiles sends file attachments to a channel with an optional bold
// caption. Failures are reported by sendWithBackoff, so there is nothing for a
// caller to inspect.
func (b *Bot) channelSendFiles(channelID, caption, what string, files []fileEntry) {
	if len(files) == 0 {
		return
	}
	payload := &discordgo.MessageSend{}
	if caption != "" {
		payload.Content = "**" + caption + "**"
	}
	b.sendWithBackoff(channelID, what, files, payload)
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
func (b *Bot) editInteractionFiles(s *discordgo.Session, i *discordgo.InteractionCreate, caption string, files []fileEntry) {
	edit := &discordgo.WebhookEdit{Files: entriesToFiles(files)}
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

// editInteractionEmbed edits the deferred interaction response with a single embed.
func (b *Bot) editInteractionEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	embeds := []*discordgo.MessageEmbed{embed}
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds}); err != nil {
		b.l.Error(fmt.Errorf("editInteractionEmbed: %w", err))
	}
}

// ---------------------------------------------------------------------------
// Verbose logging helper
// ---------------------------------------------------------------------------

// vlog emits an info log only when DISCORD_VERBOSE_LOG is enabled.
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

// TriggerScheduleNow sends a random album immediately to channelID.
func (b *Bot) TriggerScheduleNow(ctx context.Context, channelID string, historySize int) (entity.ManualScheduleTriggerResult, error) {
	ch := strings.TrimSpace(channelID)
	if ch == "" {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("send channel is not configured")
	}
	// Manual triggers are not tied to a rule, so reuse whichever scheduled rule
	// targets this channel to preview the real styling.
	style, ruleName := b.baseStyleForChannel(ctx, ch)
	return b.doScheduledSend(sendContext{RuleName: ruleName, ChannelID: ch}, historySize, style)
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
	style, ruleName := b.baseStyleForChannel(ctx, ch)
	msg := b.deliverAlbum(ctx, ch, album, sendContext{Test: true, RuleName: ruleName, ChannelID: ch}, style)
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

// SendRuleTest posts a preview of one album styled exactly as ruleID would
// style it, so the Schedule page can show what a rule actually produces —
// including event rules, which otherwise only fire from a sync.
func (b *Bot) SendRuleTest(ctx context.Context, ruleID int64, albumID int) (entity.ManualScheduleTriggerResult, error) {
	rule, err := b.rulesUC.Get(ctx, ruleID)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}
	ch := strings.TrimSpace(rule.ChannelID)
	if ch == "" {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("rule %d has no channel", ruleID)
	}

	album, err := b.pickTestAlbum(ctx, albumID, rule.HistorySize)
	if err != nil {
		return entity.ManualScheduleTriggerResult{}, err
	}

	style := entity.MergeMessageStyle(b.appStyle(ctx), rule.Style())
	msg := b.deliverAlbum(ctx, ch, album, sendContext{Test: true, RuleName: rule.Name, ChannelID: ch}, style)
	if msg == nil {
		return entity.ManualScheduleTriggerResult{}, fmt.Errorf("failed to send rule preview (see server logs)")
	}
	return entity.ManualScheduleTriggerResult{
		Triggered: true,
		AlbumID:   album.ID,
		AlbumName: album.Name,
		ChannelID: ch,
		MessageID: msg.ID,
	}, nil
}

// pickTestAlbum resolves the album a rule preview should use: the requested one
// when given, otherwise a random album (anti-repeat aware, like a real send).
func (b *Bot) pickTestAlbum(ctx context.Context, albumID, historySize int) (entity.Album, error) {
	if albumID > 0 {
		return b.imagesUC.GetAlbumByID(ctx, albumID)
	}
	return b.imagesUC.GetScheduledAlbum(ctx, historySize)
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
