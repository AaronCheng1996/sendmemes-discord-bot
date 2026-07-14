package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

// maxSyncNotifyMessages caps how many per-album notification messages one sync
// run may post; the remainder is summarized in a single extra message.
const maxSyncNotifyMessages = 20

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
	report, err := b.syncUC.SyncImages(ctx)
	if err != nil {
		b.l.Error(fmt.Errorf("doSync: %w", err))
		return
	}
	b.l.Info("pCloud sync completed")
	b.notifySyncEvents(ctx, report)
}

// notifySyncEvents posts one Discord message per album with newly discovered
// content to the configured notify channel. Nothing is sent when no channel is
// configured or when the run was the initial import (avoids flooding).
func (b *Bot) notifySyncEvents(ctx context.Context, report entity.SyncReport) {
	if len(report.Events) == 0 {
		return
	}
	if report.InitialImport {
		b.vlog("sync notify: initial import (%d albums), skipping Discord notifications", len(report.Events))
		return
	}
	effective, err := b.settingsUC.GetEffectiveSchedule(ctx, b.cfg.Discord.GuildID)
	if err != nil {
		b.l.Error(fmt.Errorf("notifySyncEvents GetEffectiveSchedule: %w", err))
		return
	}
	channelID := strings.TrimSpace(effective.NotifyChannelID)
	if channelID == "" {
		return
	}

	for i, ev := range report.Events {
		if i >= maxSyncNotifyMessages {
			rest := len(report.Events) - maxSyncNotifyMessages
			if _, serr := b.session.ChannelMessageSend(channelID, fmt.Sprintf("…and %d more album(s) with new content.", rest)); serr != nil {
				b.l.Error(fmt.Errorf("notifySyncEvents summary send: %w", serr))
			}
			break
		}
		if _, serr := b.session.ChannelMessageSend(channelID, formatSyncEventMessage(ev)); serr != nil {
			b.l.Error(fmt.Errorf("notifySyncEvents send album %q: %w", ev.AlbumName, serr))
		}
	}
	b.vlog("sync notify: posted %d notification(s) to channel %s", min(len(report.Events), maxSyncNotifyMessages), channelID)
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

func (b *Bot) runSendScheduler() {
	const retryInterval = time.Minute
	for {
		effective, err := b.settingsUC.GetEffectiveSchedule(context.Background(), b.cfg.Discord.GuildID)
		if err != nil {
			b.l.Error(fmt.Errorf("runSendScheduler load effective: %w", err))
			select {
			case <-time.After(retryInterval):
				continue
			case <-b.stopCh:
				return
			}
		}

		interval := effective.SendIntervalDuration
		if interval <= 0 {
			interval = parseDuration(effective.SendInterval, 0)
		}
		if interval <= 0 || effective.SendChannelID == "" {
			b.l.Info("scheduled send disabled (no channel ID or invalid interval)")
			select {
			case <-time.After(retryInterval):
				continue
			case <-b.stopCh:
				return
			}
		}

		select {
		case <-time.After(interval):
			_, _ = b.doScheduledSend(effective.SendChannelID, effective.SendHistorySize)
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
