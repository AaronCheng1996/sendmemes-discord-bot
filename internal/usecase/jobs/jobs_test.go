package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
)

// waitForTerminal polls the manager until the job with id is no longer running,
// failing the test if it does not settle in time.
func waitForTerminal(t *testing.T, m *Manager, id string) entity.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range m.List() {
			if j.ID == id && j.Status != entity.JobStatusRunning {
				return j
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach a terminal state", id)
	return entity.Job{}
}

func TestStartInitialStateRunning(t *testing.T) {
	m := New()
	block := make(chan struct{})
	job := m.Start(entity.JobKindSync, "pcloud", func(context.Context) (map[string]any, error) {
		<-block // keep the job running until we release it
		return nil, nil
	})

	if job.Status != entity.JobStatusRunning {
		t.Fatalf("initial status = %q, want running", job.Status)
	}
	if job.ID == "" {
		t.Fatal("job ID is empty")
	}
	if job.FinishedAt != nil {
		t.Fatal("running job should have no FinishedAt")
	}
	close(block)
	waitForTerminal(t, m, job.ID)
}

func TestStartSucceeded(t *testing.T) {
	m := New()
	job := m.Start(entity.JobKindSendTest, "album A", func(context.Context) (map[string]any, error) {
		return map[string]any{"album_name": "album A"}, nil
	})
	done := waitForTerminal(t, m, job.ID)

	if done.Status != entity.JobStatusSucceeded {
		t.Fatalf("status = %q, want succeeded", done.Status)
	}
	if done.Error != "" {
		t.Fatalf("succeeded job Error = %q, want empty", done.Error)
	}
	if done.Result["album_name"] != "album A" {
		t.Fatalf("result = %v, want album_name=album A", done.Result)
	}
	if done.FinishedAt == nil {
		t.Fatal("finished job should have FinishedAt")
	}
}

func TestStartFailed(t *testing.T) {
	m := New()
	job := m.Start(entity.JobKindScheduleSend, "chan", func(context.Context) (map[string]any, error) {
		return nil, errors.New("boom")
	})
	done := waitForTerminal(t, m, job.ID)

	if done.Status != entity.JobStatusFailed {
		t.Fatalf("status = %q, want failed", done.Status)
	}
	if done.Error != "boom" {
		t.Fatalf("Error = %q, want boom", done.Error)
	}
}

func TestListNewestFirst(t *testing.T) {
	m := New()
	first := m.Start(entity.JobKindSync, "1", func(context.Context) (map[string]any, error) { return nil, nil })
	waitForTerminal(t, m, first.ID)
	second := m.Start(entity.JobKindSync, "2", func(context.Context) (map[string]any, error) { return nil, nil })
	waitForTerminal(t, m, second.ID)

	list := m.List()
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("order = [%s, %s], want [%s, %s] (newest first)", list[0].ID, list[1].ID, second.ID, first.ID)
	}
}

func TestListCappedAt50(t *testing.T) {
	m := New()
	var lastID string
	for i := 0; i < 60; i++ {
		j := m.Start(entity.JobKindSync, "x", func(context.Context) (map[string]any, error) { return nil, nil })
		waitForTerminal(t, m, j.ID)
		lastID = j.ID
	}

	list := m.List()
	if len(list) != maxJobs {
		t.Fatalf("len(list) = %d, want %d", len(list), maxJobs)
	}
	// The newest job must be retained; the 10 oldest must have been dropped.
	if list[0].ID != lastID {
		t.Fatalf("newest ID = %s, want %s", list[0].ID, lastID)
	}
}
