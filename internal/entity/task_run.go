package entity

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Task run statuses.
const (
	TaskRunRunning   = "running"
	TaskRunSucceeded = "succeeded"
	TaskRunFailed    = "failed"
)

// Known task run sources. The column is deliberately free-form — an ingest
// client names itself — so these are the ones this process writes, not a
// closed set.
const (
	TaskRunSourceScheduledSend = "scheduled_send"
	TaskRunSourceSync          = "sync"
)

// taskRunLimits bound what one row may carry, so a misbehaving ingest client
// cannot bloat the table a field at a time.
const (
	maxTaskRunFieldLen  = 200
	maxTaskRunSummary   = 2000
	maxTaskRunErrorLen  = 4000
	maxTaskRunDetailLen = 16 * 1024
)

// TaskRun is one execution of something worth reviewing later: a scheduled
// send, a sync run, or a crawler pass reported over the ingest API.
//
// It is run-shaped rather than line-shaped on purpose. The dashboard shows one
// row per run with Summary as its single line, and expands Detail for whoever
// wants the steps — which is why the writer produces the row, instead of
// something trying to group log lines back into runs afterwards.
type TaskRun struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
	Task   string `json:"task,omitempty"`
	Status string `json:"status"`

	// StartedAt is when the work began; a client reporting a finished run may
	// backdate it. Zero means "now" at insert time.
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Summary is the one line the collapsed row shows.
	Summary string `json:"summary,omitempty"`

	// Detail is the expandable payload: counts, steps, retries, whatever the
	// writer thought was worth keeping. Stored as JSON, never interpreted here.
	Detail    map[string]any `json:"detail,omitempty"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// ParseTaskRunStatus normalizes and validates a status string.
func ParseTaskRunStatus(s string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(s))
	switch status {
	case TaskRunRunning, TaskRunSucceeded, TaskRunFailed:
		return status, nil
	default:
		return "", fmt.Errorf("invalid task run status: %q (want running, succeeded, or failed)", s)
	}
}

// Normalize validates and trims a run prior to persistence. It fills the
// obvious defaults — a missing start time, and the finish time a terminal
// status implies — so an ingest client can report a completed run in one call
// without spelling every field out.
func (r *TaskRun) Normalize() error {
	r.Source = truncate(strings.TrimSpace(r.Source), maxTaskRunFieldLen)
	if r.Source == "" {
		return fmt.Errorf("task run source is required")
	}

	r.Task = truncate(strings.TrimSpace(r.Task), maxTaskRunFieldLen)
	status, err := ParseTaskRunStatus(r.Status)
	if err != nil {
		return err
	}

	r.Status = status
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}

	if r.Status != TaskRunRunning && r.FinishedAt == nil {
		finished := time.Now().UTC()
		r.FinishedAt = &finished
	}

	if r.Status == TaskRunRunning {
		r.FinishedAt = nil
	}

	r.Summary = truncate(strings.TrimSpace(r.Summary), maxTaskRunSummary)
	r.Error = truncate(strings.TrimSpace(r.Error), maxTaskRunErrorLen)

	return r.checkDetailSize()
}

// checkDetailSize rejects an oversized detail payload.
//
// Detail is the only field a client can grow without bound, and it lands in a
// jsonb column. This rejects where the other fields truncate, on purpose: a
// clipped name is still recognizably that name, whereas half a JSON object is
// worthless. A client that has outgrown the cap should hear about it rather
// than discover months later that its payloads were being quietly discarded.
func (r *TaskRun) checkDetailSize() error {
	if len(r.Detail) == 0 {
		return nil
	}

	raw, err := json.Marshal(r.Detail)
	if err != nil {
		return fmt.Errorf("task run detail is not serializable: %w", err)
	}
	if len(raw) > maxTaskRunDetailLen {
		return fmt.Errorf("task run detail is %d bytes, over the %d-byte limit", len(raw), maxTaskRunDetailLen)
	}

	return nil
}

// Duration is how long the run took, or zero while it is still running.
func (r TaskRun) Duration() time.Duration {
	if r.FinishedAt == nil {
		return 0
	}

	return r.FinishedAt.Sub(r.StartedAt)
}

// truncate cuts s to at most n bytes, on a rune boundary so the result stays
// valid UTF-8 (the DB columns are text, and a split rune would store as garbage).
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	cut := n
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}

	return s[:cut]
}

// utf8RuneStart reports whether b begins a UTF-8 rune (i.e. is not a
// continuation byte).
func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
