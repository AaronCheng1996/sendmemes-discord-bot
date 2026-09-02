package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/schedulespec"
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
		// The interval is re-read every round so runtime edits from the
		// Connection page take effect on the next tick. It may be a Go
		// duration or a cron expression.
		wait := b.nextSyncWait(intervalStr)
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			b.doSync()
		case <-b.stopCh:
			timer.Stop()
			return
		}
	}
}

// nextSyncWait resolves how long to wait before the next sync from intervalStr,
// falling back to the env default and finally to one hour when parsing fails.
func (b *Bot) nextSyncWait(intervalStr string) time.Duration {
	spec, err := schedulespec.Parse(intervalStr)
	if err != nil {
		b.l.Error(fmt.Errorf("runSyncScheduler: invalid sync interval %q, using env default: %w", intervalStr, err))
		spec, err = schedulespec.Parse(b.cfg.PCloud.SyncInterval)
		if err != nil {
			return time.Hour
		}
	}
	next := spec.Next(time.Now())
	if next.IsZero() {
		return time.Hour
	}
	if wait := time.Until(next); wait > 0 {
		return wait
	}
	return time.Hour
}

func (b *Bot) doSync() {
	ctx := context.Background()
	b.l.Info("pCloud sync started")
	started := time.Now().UTC()
	report, err := b.syncUC.SyncImages(ctx)
	b.recordSyncRun(ctx, "scheduled", started, &report, err)
	if err != nil {
		b.l.Error(fmt.Errorf("doSync: %w", err))
		return
	}
	b.l.Info("pCloud sync completed")
	if report.EmptyScan {
		b.l.Warn("sync found no media at all — skipped the missing-album pass (check the source configuration and credentials)")
	}
	if len(report.MissingAlbums) > 0 {
		b.l.Info("sync flagged %d album(s) as missing (hidden from the dashboard and excluded from scheduled sends until their folder returns): %s",
			len(report.MissingAlbums), strings.Join(report.MissingAlbums, ", "))
	}
	if len(report.Notices) > 0 {
		b.vlog("sync recorded %d activity-log notice(s) (removals/renames); not delivered to Discord", len(report.Notices))
	}
	b.notifySyncEvents(ctx, report)
}

// recordSyncRun writes one row to the durable run log describing a sync,
// whichever way it was started. The counts it keeps are the ones nobody can
// reconstruct from the albums table afterwards: what this particular run found,
// retired and renamed.
func (b *Bot) recordSyncRun(ctx context.Context, trigger string, started time.Time, report *entity.SyncReport, runErr error) {
	newImages, newVideos := 0, 0
	for i := range report.Events {
		newImages += report.Events[i].NewImages
		newVideos += report.Events[i].NewVideos
	}
	removedImages, removedVideos := 0, 0
	for i := range report.Notices {
		removedImages += report.Notices[i].RemovedImages
		removedVideos += report.Notices[i].RemovedVideos
	}

	detail := map[string]any{
		"trigger":        trigger,
		"albums_changed": len(report.Events),
		"new_images":     newImages,
		"new_videos":     newVideos,
		"removed_images": removedImages,
		"removed_videos": removedVideos,
		"missing_albums": report.MissingAlbums,
		"notices":        len(report.Notices),
		"empty_scan":     report.EmptyScan,
		"initial_import": report.InitialImport,
	}

	status := entity.TaskRunSucceeded
	summary := fmt.Sprintf("%d album(s) changed, +%d image(s) +%d video(s), -%d image(s) -%d video(s)",
		len(report.Events), newImages, newVideos, removedImages, removedVideos)
	switch {
	case runErr != nil:
		status = entity.TaskRunFailed
		summary = "Sync failed"
	case report.EmptyScan:
		// Not an error — the source answered — but the run did nothing and the
		// missing pass was skipped, which is worth spotting in the log.
		summary = "Source returned no media at all; missing-album pass skipped"
	}

	b.recordRun(ctx, &entity.TaskRun{
		Source:    entity.TaskRunSourceSync,
		Task:      trigger,
		Status:    status,
		StartedAt: started,
		Summary:   summary,
		Detail:    detail,
		Error:     errText(runErr),
	})
}

// taskRunSweepInterval is how often expired run rows are swept. The retention
// window is measured in weeks, so a sweep a few times a day is plenty; running
// it more often would only add queries.
const taskRunSweepInterval = 6 * time.Hour

// runTaskRunRetention drops run rows past their retention window. Without it
// the table grows without bound — a crawler reporting every pass is exactly the
// client that would fill it.
func (b *Bot) runTaskRunRetention() {
	if b.runsUC == nil {
		return
	}
	sweep := func() {
		n, err := b.runsUC.Prune(context.Background())
		if err != nil {
			b.l.Error(fmt.Errorf("runTaskRunRetention: %w", err))
			return
		}
		if n > 0 {
			b.vlog("task run retention: pruned %d expired run(s)", n)
		}
	}

	// Sweep once at startup: an instance that only runs for a few hours a day
	// would otherwise never reach a tick.
	sweep()

	ticker := time.NewTicker(taskRunSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sweep()
		case <-b.stopCh:
			return
		}
	}
}

// recordRun appends one finished run to the durable log. Recording is
// best-effort by design: a database hiccup must not turn a successful send into
// a failure, so it is logged and swallowed.
func (b *Bot) recordRun(ctx context.Context, run *entity.TaskRun) {
	if b.runsUC == nil {
		return
	}
	if _, err := b.runsUC.Record(ctx, *run); err != nil {
		b.l.Error(fmt.Errorf("recordRun %s/%q: %w", run.Source, run.Task, err))
	}
}

// errText renders an error for a run row, empty when there was none.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
		trigger := entity.SyncEventTriggerType(ev.EventType)
		if trigger == "" {
			// Removals and renames have no matching rule trigger; report.Notices
			// already keeps them out of this loop, so this only guards a future
			// event type that forgets to.
			continue
		}
		rules, err := b.rulesUC.ListActiveByTrigger(ctx, trigger)
		if err != nil {
			b.l.Error(fmt.Errorf("notifySyncEvents ListActiveByTrigger: %w", err))
			continue
		}
		for _, rule := range rules {
			// A rule scoped to part of the library ignores everything outside
			// it, which is what lets one channel carry the crawler's finds and
			// another carry the rest.
			if !rule.AlbumFilter.Matches(ev.AlbumPath) {
				b.vlog("sync notify: rule %d skips %q (albums=%s, path=%q)",
					rule.ID, ev.AlbumName, rule.AlbumFilter.Describe(), ev.AlbumPath)
				continue
			}
			b.postDiscoveredMedia(ctx, rule, ev)
		}
	}
}

// postDiscoveredMedia posts an event's newly discovered media to channelID.
//
// Everything that fits the channel's upload budget goes into one size-fitted
// attachment message (up to albumBatchSize), videos included — Discord plays an
// uploaded clip inline, so a small one belongs with the images rather than
// behind a link. Only a video too large to upload falls back to a permanent
// pCloud public link. Falls back to a plain text summary when nothing can be
// resolved.
func (b *Bot) postDiscoveredMedia(ctx context.Context, rule entity.DeliveryRule, ev entity.SyncEvent) {
	channelID := rule.ChannelID
	// Discovery posts honour the same three-layer style as scheduled sends, minus
	// the album layer (a SyncEvent has no send config). An unset body keeps the
	// generated "N new images" summary.
	style := entity.MergeMessageStyle(b.appStyle(ctx), rule.Style())
	sc := sendContext{RuleName: rule.Name, ChannelID: channelID}
	caption := formatSyncEventMessage(ev)
	// SyncEvent carries only id+name, not a full Album (no send mode/rating to
	// look up here) — albumEmbed degrades gracefully for the missing fields.
	album := entity.Album{ID: ev.AlbumID, Name: ev.AlbumName}
	// The album totals are what a caption's "now at N files" line reports, so
	// they are read once per notification rather than per message below.
	counts := b.albumCounts(ctx, ev.AlbumID)

	budget := b.uploadLimit(channelID)
	attachable, linkOnly := splitByUploadability(ev.NewMedia, budget)

	posted := false

	// Everything small enough travels as an attachment, clips alongside stills.
	if len(attachable) > 0 {
		pool := attachable
		if len(pool) > albumPoolSize {
			pool = pool[:albumPoolSize]
		}
		entries, err := b.downloadPool(ctx, pool)
		if err != nil {
			b.l.Error(fmt.Errorf("postDiscoveredMedia downloadPool %q: %w", ev.AlbumName, err))
		} else if selected := fitToLimit(b.l, entries, albumBatchSize, budget); len(selected) > 0 {
			desc := caption
			if len(attachable) > len(selected) {
				desc += fmt.Sprintf(" (showing %d of %d)", len(selected), len(attachable))
			}
			files := selected
			msg := syncMessage(style, ev, album, len(selected), counts, sc, desc)
			if b.sendStyled(channelID, album, msg, b.resolveThumbURL(ctx, attachable), firstEmbeddableName(files), files, nil) != nil {
				posted = true
			}
		}
	}

	// Only what Discord will not take gets a link instead.
	if len(linkOnly) > 0 {
		links := make([]string, 0, maxNotifyVideoLinks)
		for i, v := range linkOnly {
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
			if len(linkOnly) > len(links) {
				fmt.Fprintf(&sb, "\n…and %d more too large to upload", len(linkOnly)-len(links))
			}
			msg := syncMessage(style, ev, album, len(links), counts, sc, sb.String())
			if b.sendStyled(channelID, album, msg, "", "", nil, nil) != nil {
				posted = true
			}
		}
	}

	// Nothing resolvable (e.g. counts-only event) — post the text summary.
	if !posted {
		msg := syncMessage(style, ev, album, 0, counts, sc, caption)
		if b.sendStyled(channelID, album, msg, "", "", nil, nil) == nil {
			b.l.Error(fmt.Errorf("postDiscoveredMedia fallback %q: failed to send", ev.AlbumName))
		}
	}
}

// syncMessage applies a discovery notification's resolved style: the rule's
// title/embed preference are honoured, while the body defaults to the generated
// summary unless the rule explicitly overrides it.
func syncMessage(style entity.MessageStyle, ev entity.SyncEvent, album entity.Album, shown int, counts albumCounts, sc sendContext, summary string) renderedMessage {
	tokens := discoveryTokens(album, ev, shown, counts, sc)
	return renderMessage(style, tokens, summary, sc.Test)
}

// splitByUploadability divides newly discovered media into what can be attached
// to a message and what has to be linked instead.
//
// Only videos are ever linked, and only when their recorded size rules out an
// upload. The size is checked here rather than after downloading because a clip
// too large to send is also the one most expensive to fetch and throw away.
// An unrecorded size (0) counts as too large: sync stamps every file it
// ingests, so a missing one means something unusual enough not to gamble a
// download on.
func splitByUploadability(media []entity.Image, budget int) (attachable, linkOnly []entity.Image) {
	for _, m := range media {
		if m.Kind == entity.MediaKindVideo && (m.SizeBytes <= 0 || m.SizeBytes > int64(budget)) {
			linkOnly = append(linkOnly, m)
			continue
		}
		attachable = append(attachable, m)
	}
	return attachable, linkOnly
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

// scheduledRuleSig decides whether a running rule goroutine still matches its
// database row. It hashes updated_at rather than the individual fields: the
// goroutine captures the whole rule by value, so ANY edit — interval, channel,
// styling, album filter — has to restart it. Listing fields here is how editing
// a rule's styling used to silently do nothing until the next restart.
func scheduledRuleSig(r entity.DeliveryRule) string {
	return fmt.Sprintf("%d|%s", r.ID, r.UpdatedAt.UTC().Format(time.RFC3339Nano))
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
		b.vlog("schedule manager: started rule %d (interval=%s channel=%s albums=%s)",
			rule.ID, rule.SendInterval, rule.ChannelID, rule.AlbumFilter.Describe())
	}

	for id, h := range running {
		if _, ok := seen[id]; !ok {
			h.cancel()
			delete(running, id)
			b.vlog("schedule manager: stopped rule %d (removed or disabled)", id)
		}
	}
}

// runScheduledRule fires one scheduled rule on its schedule (a Go duration or a
// cron expression) until its context or the bot's stop channel is canceled.
func (b *Bot) runScheduledRule(ctx context.Context, rule entity.DeliveryRule) {
	spec, err := schedulespec.Parse(rule.SendInterval)
	if err != nil {
		b.l.Info("schedule rule %d disabled (invalid interval %q): %v", rule.ID, rule.SendInterval, err)
		return
	}
	for {
		next := spec.Next(time.Now())
		if next.IsZero() {
			b.l.Info("schedule rule %d disabled (interval %q never fires)", rule.ID, rule.SendInterval)
			return
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			_, _ = b.doScheduledSend(
				sendContext{RuleName: rule.Name, ChannelID: rule.ChannelID},
				rule.HistorySize,
				entity.MergeMessageStyle(b.appStyle(context.Background()), rule.Style()),
				rule.AlbumFilter)
		case <-ctx.Done():
			timer.Stop()
			return
		case <-b.stopCh:
			timer.Stop()
			return
		}
	}
}

// doScheduledSend runs one scheduled push and records it in the durable log, so
// "how did the 2pm send go" is one row on the System log page rather than a
// scroll through the process output.
func (b *Bot) doScheduledSend(sc sendContext, historySize int, style entity.MessageStyle, filter entity.AlbumPathFilter) (entity.ManualScheduleTriggerResult, error) {
	ctx := context.Background()
	started := time.Now().UTC()

	result, detail, err := b.scheduledSend(ctx, sc, historySize, style, filter)
	detail["history_size"] = historySize
	detail["album_scope"] = filter.Describe()
	detail["channel_id"] = sc.ChannelID

	task := sc.RuleName
	if task == "" {
		// A manual trigger belongs to no rule; name it by where it landed.
		task = "channel " + sc.ChannelID
	}

	status, summary := entity.TaskRunSucceeded, fmt.Sprintf("Posted %q to %s", result.AlbumName, sc.ChannelID)
	switch {
	case err != nil:
		status = entity.TaskRunFailed
		summary = "Scheduled send failed"
		if result.AlbumName != "" {
			summary = fmt.Sprintf("Scheduled send of %q failed", result.AlbumName)
		}
	case !result.Triggered:
		// The album was picked but nothing reached Discord — an empty album, or
		// every attachment over budget. No error came back, so without this the
		// run would read as a success that posted nothing.
		status = entity.TaskRunFailed
		summary = fmt.Sprintf("Selected %q but nothing was delivered", result.AlbumName)
	}

	b.recordRun(ctx, &entity.TaskRun{
		Source:    entity.TaskRunSourceScheduledSend,
		Task:      task,
		Status:    status,
		StartedAt: started,
		Summary:   summary,
		Detail:    detail,
		Error:     errText(err),
	})

	return result, err
}

// scheduledSend is doScheduledSend's actual work, returning the detail payload
// its run row carries alongside the result.
func (b *Bot) scheduledSend(ctx context.Context, sc sendContext, historySize int, style entity.MessageStyle, filter entity.AlbumPathFilter) (entity.ManualScheduleTriggerResult, map[string]any, error) {
	detail := map[string]any{}
	channelID := sc.ChannelID
	b.vlog("scheduled send: selecting album (history=%d albums=%s)", historySize, filter.Describe())
	album, err := b.imagesUC.GetScheduledAlbum(ctx, historySize, filter)
	if err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend GetScheduledAlbum: %w", err))
		return entity.ManualScheduleTriggerResult{}, detail, err
	}
	detail["album_id"] = album.ID
	detail["album_name"] = album.Name
	detail["send_mode"] = string(album.SendMode)

	b.vlog("scheduled send: album=%q id=%d mode=%s sending to channel %s", album.Name, album.ID, album.SendMode, channelID)
	msg := b.deliverAlbum(ctx, channelID, album, sc, style)
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
		detail["message_id"] = msg.ID
	}
	// Mark sent regardless of delivery outcome so a broken album is not re-picked
	// on every tick.
	if err := b.imagesUC.MarkAlbumSent(ctx, album.ID); err != nil {
		b.l.Error(fmt.Errorf("doScheduledSend MarkAlbumSent: %w", err))
		return result, detail, err
	}

	return result, detail, nil
}
