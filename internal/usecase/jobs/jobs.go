// Package jobs provides an in-memory manager for background delivery jobs.
//
// The manager holds only the most recent jobs and forgets everything on
// restart. That is acceptable: the jobs it tracks (send-test, scheduled send,
// pCloud sync) are short-lived and best-effort, and their durable side effects
// (Discord messages, audit rows) are recorded independently.
package jobs

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

// maxJobs caps how many recent jobs the manager keeps.
const maxJobs = 50

// Manager tracks recently started background jobs. It is safe for concurrent use.
type Manager struct {
	mu     sync.Mutex
	seq    int64
	recent []*entity.Job // oldest first, capped at maxJobs
}

// New creates an empty job manager.
func New() *Manager {
	return &Manager{}
}

// Start records a new running job, runs fn in a goroutine, and returns the job
// in its initial running state. When fn returns, the job transitions to
// succeeded (carrying fn's result) or failed (carrying fn's error). fn receives
// a background context so it outlives the request that started it.
func (m *Manager) Start(kind entity.JobKind, label string, fn func(context.Context) (map[string]any, error)) entity.Job {
	m.mu.Lock()
	m.seq++
	job := &entity.Job{
		ID:        strconv.FormatInt(m.seq, 10),
		Kind:      kind,
		Label:     label,
		Status:    entity.JobStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	m.recent = append(m.recent, job)
	if len(m.recent) > maxJobs {
		m.recent = m.recent[len(m.recent)-maxJobs:]
	}
	snapshot := *job
	m.mu.Unlock()

	go func() {
		result, err := fn(context.Background())

		m.mu.Lock()
		defer m.mu.Unlock()
		finished := time.Now().UTC()
		job.FinishedAt = &finished
		if err != nil {
			job.Status = entity.JobStatusFailed
			job.Error = err.Error()
		} else {
			job.Status = entity.JobStatusSucceeded
			job.Result = result
		}
	}()

	return snapshot
}

// List returns the tracked jobs newest first (up to maxJobs).
func (m *Manager) List() []entity.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]entity.Job, 0, len(m.recent))
	for i := len(m.recent) - 1; i >= 0; i-- {
		out = append(out, *m.recent[i])
	}
	return out
}
