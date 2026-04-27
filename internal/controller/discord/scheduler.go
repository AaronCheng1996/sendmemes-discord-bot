package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

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
	imgs, albumID, err := b.imagesUC.GetScheduledAlbumImages(ctx, historySize, albumPoolSize)
	if err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend GetScheduledAlbumImages: %w", err))
		return entity.ManualScheduleTriggerResult{}, err
	}
	albumName := albumNameFrom(imgs)
	b.vlog("scheduled send: album=%q id=%d sending to channel %s", albumName, albumID, channelID)
	msg := b.sendAlbumToChannel(ctx, b.session, channelID, albumName, imgs)
	result := entity.ManualScheduleTriggerResult{
		Triggered: msg != nil,
		AlbumID:   albumID,
		AlbumName: albumName,
		ChannelID: channelID,
	}
	if msg != nil {
		b.trackScheduledMsg(msg.ID, albumID)
		b.vlog("scheduled send: completed album=%q messageID=%s", albumName, msg.ID)
		result.MessageID = msg.ID
	}
	if err := b.imagesUC.MarkAlbumSent(ctx, albumID); err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend MarkAlbumSent: %w", err))
		return result, err
	}
	return result, nil
}
