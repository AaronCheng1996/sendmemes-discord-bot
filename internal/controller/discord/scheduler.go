package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

const (
	// maxSyncNotifyMessages caps how many discovery events one sync run notifies
	// about, guarding against a flood when many albums change at once.
	maxSyncNotifyMessages = 20
	// scheduleReconcileInterval is how often the manager reloads scheduled rules
	// from the DB so UI/slash changes take effect without a restart.
	scheduleReconcileInterval = 30 * time.Second
	// maxNotifyVideoLinks caps how many video links one discovery notification posts.
	maxNotifyVideoLinks = 5
)

// ---------------------------------------------------------------------------
// pCloud sync scheduler
// ---------------------------------------------------------------------------

func (b *Bot) runSyncScheduler() {
	hasCredentials := b.cfg.PCloud.AccessToken != "" || b.cfg.PCloud.Username != ""
	if !hasCredentials {
		b.l.Info("pCloud sync disabled (no credentials configured)")
		return
	}
	b.doSync()
	for {
		intervalStr, err := b.appSettingsUC.GetSyncInterval(context.Background())
		if err != nil {
			b.l.Error(fmt.Errorf("runSyncScheduler GetSyncInterval: %w", err))
			intervalStr = b.cfg.PCloud.SyncInterval
		}
		interval := parseDuration(intervalStr, time.Hour)
		select {
		case <-time.After(interval):
			b.doSync()
		case <-b.stopCh:
			return
		}
	}
}

func (b *Bot) doSync() {
	ctx := context.Background()
	b.l.Info("pCloud sync started")
	report, err := b.syncUC.SyncImages(ctx)
	if err != nil {
		b.l.Error(fmt.Errorf("doSync: %w", err))
		return
	}
	b.l.Info("pCloud sync completed")
	b.notifySyncEvents(ctx, report)
}

// notifySyncEvents posts discovered content to every enabled delivery rule whose
// trigger matches each event (new_album / new_files). Nothing is sent for the
// initial import (avoids flooding a freshly seeded database).
func (b *Bot) notifySyncEvents(ctx context.Context, report entity.SyncReport) {
	if len(report.Events) == 0 {
		return
	}
	if report.InitialImport {
		b.vlog("sync notify: initial import (%d albums), skipping Discord notifications", len(report.Events))
		return
	}

	for i, ev := range report.Events {
		if i >= maxSyncNotifyMessages {
			b.vlog("sync notify: reached %d-event cap, skipping the rest", maxSyncNotifyMessages)
			break
		}
		rules, err := b.rulesUC.ListActiveByTrigger(ctx, entity.SyncEventTriggerType(ev.EventType))
		if err != nil {
			b.l.Error(fmt.Errorf("notifySyncEvents ListActiveByTrigger: %w", err))
			continue
		}
		for _, rule := range rules {
			b.postDiscoveredMedia(ctx, rule.ChannelID, ev)
		}
	}
}

// postDiscoveredMedia posts an event's newly discovered media to channelID: new
// images are merged into one size-fitted attachment message (up to
// albumBatchSize), new videos are posted as permanent pCloud public links (never
// uploaded). Falls back to a plain text summary when nothing can be resolved.
func (b *Bot) postDiscoveredMedia(ctx context.Context, channelID string, ev entity.SyncEvent) {
	caption := formatSyncEventMessage(ev)

	var images, videos []entity.Image
	for _, m := range ev.NewMedia {
		if m.Kind == entity.MediaKindVideo {
			videos = append(videos, m)
		} else {
			images = append(images, m)
		}
	}

	posted := false

	// New images: one merged attachment message.
	if len(images) > 0 {
		pool := images
		if len(pool) > albumPoolSize {
			pool = pool[:albumPoolSize]
		}
		entries, err := b.downloadPool(ctx, pool)
		if err != nil {
			b.l.Error(fmt.Errorf("postDiscoveredMedia downloadPool %q: %w", ev.AlbumName, err))
		} else if selected := fitToLimit(b.l, entries, albumBatchSize, discordMsgLimit); len(selected) > 0 {
			content := caption
			if len(images) > len(selected) {
				content += fmt.Sprintf(" (showing %d of %d)", len(selected), len(images))
			}
			if b.channelSendFilesContent(channelID, content, entriesToFiles(selected)) != nil {
				posted = true
			}
		}
	}

	// New videos: permanent public links only (per configuration, never uploaded).
	if len(videos) > 0 {
		links := make([]string, 0, maxNotifyVideoLinks)
		for i, v := range videos {
			if i >= maxNotifyVideoLinks {
				break
			}
			url, err := b.imagesUC.ResolvePublicURL(ctx, v)
			if err != nil {
				b.l.Error(fmt.Errorf("postDiscoveredMedia ResolvePublicURL %q: %w", ev.AlbumName, err))
				continue
			}
			links = append(links, url)
		}
		if len(links) > 0 {
			var sb strings.Builder
			if !posted {
				sb.WriteString(caption)
				sb.WriteString("\n")
			}
			sb.WriteString(strings.Join(links, "\n"))
			if len(videos) > len(links) {
				fmt.Fprintf(&sb, "\n…and %d more video(s)", len(videos)-len(links))
			}
			if _, err := b.session.ChannelMessageSend(channelID, sb.String()); err != nil {
				b.l.Error(fmt.Errorf("postDiscoveredMedia video links %q: %w", ev.AlbumName, err))
			} else {
				posted = true
			}
		}
	}

	// Nothing resolvable (e.g. counts-only event) — post the text summary.
	if !posted {
		if _, err := b.session.ChannelMessageSend(channelID, caption); err != nil {
			b.l.Error(fmt.Errorf("postDiscoveredMedia fallback %q: %w", ev.AlbumName, err))
		}
	}
}

// formatSyncEventMessage renders one sync event as a Discord message line, e.g.
// "🆕 **Name** — new album: 3 images, 1 video" or "📥 **Name** — +2 images".
func formatSyncEventMessage(ev entity.SyncEvent) string {
	var counts []string
	if ev.NewImages > 0 {
		counts = append(counts, countPhrase(ev.NewImages, "image"))
	}
	if ev.NewVideos > 0 {
		counts = append(counts, countPhrase(ev.NewVideos, "video"))
	}

	if ev.EventType == entity.SyncEventAlbumCreated {
		detail := strings.Join(counts, ", ")
		if detail == "" {
			detail = "empty"
		}
		return fmt.Sprintf("🆕 **%s** — new album: %s", ev.AlbumName, detail)
	}

	for i := range counts {
		counts[i] = "+" + counts[i]
	}
	return fmt.Sprintf("📥 **%s** — %s", ev.AlbumName, strings.Join(counts, ", "))
}

// countPhrase renders a count with a naively pluralized noun ("1 image", "3 videos").
func countPhrase(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// ---------------------------------------------------------------------------
// Scheduled-send manager (one goroutine per enabled scheduled rule)
// ---------------------------------------------------------------------------

// scheduledHandle tracks a running scheduled-rule goroutine and the rule
// signature it was started with, so the manager can detect changes.
type scheduledHandle struct {
	cancel context.CancelFunc
	sig    string
}

func scheduledRuleSig(r entity.DeliveryRule) string {
	return fmt.Sprintf("%s|%d|%v", r.SendInterval, r.HistorySize, r.ChannelID)
}

// runScheduleManager periodically reconciles the set of running scheduled-rule
// goroutines with the enabled 'scheduled' rules in the database.
func (b *Bot) runScheduleManager() {
	running := make(map[int64]scheduledHandle)
	stopAll := func() {
		for _, h := range running {
			h.cancel()
		}
	}
	defer stopAll()

	b.reconcileScheduledRules(running)

	ticker := time.NewTicker(scheduleReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.reconcileScheduledRules(running)
		case <-b.stopCh:
			return
		}
	}
}

func (b *Bot) reconcileScheduledRules(running map[int64]scheduledHandle) {
	rules, err := b.rulesUC.ListActiveByTrigger(context.Background(), entity.TriggerScheduled)
	if err != nil {
		b.l.Error(fmt.Errorf("reconcileScheduledRules: %w", err))
		return
	}

	seen := make(map[int64]struct{}, len(rules))
	for _, rule := range rules {
		seen[rule.ID] = struct{}{}
		sig := scheduledRuleSig(rule)
		if h, ok := running[rule.ID]; ok {
			if h.sig == sig {
				continue // unchanged
			}
			h.cancel() // interval/channel changed — restart
		}
		ctx, cancel := context.WithCancel(context.Background())
		running[rule.ID] = scheduledHandle{cancel: cancel, sig: sig}
		go b.runScheduledRule(ctx, rule)
		b.vlog("schedule manager: started rule %d (interval=%s channel=%s)", rule.ID, rule.SendInterval, rule.ChannelID)
	}

	for id, h := range running {
		if _, ok := seen[id]; !ok {
			h.cancel()
			delete(running, id)
			b.vlog("schedule manager: stopped rule %d (removed or disabled)", id)
		}
	}
}

// runScheduledRule fires one scheduled rule every interval until its context or
// the bot's stop channel is cancelled.
func (b *Bot) runScheduledRule(ctx context.Context, rule entity.DeliveryRule) {
	interval := parseDuration(rule.SendInterval, 0)
	if interval <= 0 {
		b.l.Info("schedule rule %d disabled (invalid interval %q)", rule.ID, rule.SendInterval)
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_, _ = b.doScheduledSend(rule.ChannelID, rule.HistorySize)
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		}
	}
}

func (b *Bot) doScheduledSend(channelID string, historySize int) (entity.ManualScheduleTriggerResult, error) {
	ctx := context.Background()
	b.vlog("scheduled send: selecting album (history=%d)", historySize)
	album, err := b.imagesUC.GetScheduledAlbum(ctx, historySize)
	if err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend GetScheduledAlbum: %w", err))
		return entity.ManualScheduleTriggerResult{}, err
	}
	b.vlog("scheduled send: album=%q id=%d mode=%s sending to channel %s", album.Name, album.ID, album.SendMode, channelID)
	msg := b.deliverAlbum(ctx, channelID, album, "")
	result := entity.ManualScheduleTriggerResult{
		Triggered: msg != nil,
		AlbumID:   album.ID,
		AlbumName: album.Name,
		ChannelID: channelID,
	}
	if msg != nil {
		b.trackScheduledMsg(msg.ID, album.ID)
		b.vlog("scheduled send: completed album=%q messageID=%s", album.Name, msg.ID)
		result.MessageID = msg.ID
	}
	// Mark sent regardless of delivery outcome so a broken album is not re-picked
	// on every tick.
	if err := b.imagesUC.MarkAlbumSent(ctx, album.ID); err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend MarkAlbumSent: %w", err))
		return result, err
	}
	return result, nil
}
