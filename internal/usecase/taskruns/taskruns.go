// Package taskruns records what each execution of a scheduled send, a sync, or
// an external client did, so the dashboard can answer "how did the 2pm push go"
// with one row instead of ten log lines.
package taskruns

import (
	"context"
	"fmt"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
)

// Retention is how long a run is kept. Runs are operational history: useful for
// a few weeks of "what happened last Tuesday", worthless after that, and
// unbounded growth is a real risk with a crawler reporting on every pass.
const Retention = 30 * 24 * time.Hour

// UseCase records and reads task runs.
type UseCase struct {
	repo repo.TaskRunsRepo
}

// New creates a task run use case.
func New(r repo.TaskRunsRepo) *UseCase {
	return &UseCase{repo: r}
}

// Record stores a run. A run reported with a terminal status is complete in one
// call — which is all a crawler needs — while status "running" opens a row for
// Complete to close later.
func (uc *UseCase) Record(ctx context.Context, run entity.TaskRun) (entity.TaskRun, error) {
	if err := run.Normalize(); err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsUseCase - Record: %w", err)
	}
	saved, err := uc.repo.Insert(ctx, run)
	if err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsUseCase - Record - Insert: %w", err)
	}

	return saved, nil
}

// Complete closes an open run with its outcome.
func (uc *UseCase) Complete(ctx context.Context, id int64, outcome entity.TaskRun) (entity.TaskRun, error) {
	// Normalize needs a source to validate against, but Complete never writes
	// one — the stored row keeps its own. Borrow a placeholder for the checks.
	outcome.Source = "-"
	if err := outcome.Normalize(); err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsUseCase - Complete: %w", err)
	}
	saved, err := uc.repo.Complete(ctx, id, outcome)
	if err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsUseCase - Complete: %w", err)
	}

	return saved, nil
}

// List returns a page of runs plus the total matching the same filters.
func (uc *UseCase) List(ctx context.Context, q repo.TaskRunListQuery, offset, limit int) ([]entity.TaskRun, int, error) {
	items, err := uc.repo.List(ctx, q, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.repo.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// Sources returns the distinct sources that have reported runs.
func (uc *UseCase) Sources(ctx context.Context) ([]string, error) {
	return uc.repo.Sources(ctx)
}

// Prune drops runs older than Retention and returns how many went.
func (uc *UseCase) Prune(ctx context.Context) (int64, error) {
	n, err := uc.repo.PruneBefore(ctx, time.Now().Add(-Retention))
	if err != nil {
		return 0, fmt.Errorf("TaskRunsUseCase - Prune: %w", err)
	}

	return n, nil
}

// Started opens a run in the "running" state, returning a finish function that
// closes it. Callers that already know the outcome should use Record with a
// terminal status instead; this is for work worth showing while it is in
// flight.
//
// Recording is best-effort: a database hiccup must not take down the send it is
// only describing, so failures come back as an error the caller may log and
// ignore, and the returned finisher tolerates a run that was never opened.
func (uc *UseCase) Started(ctx context.Context, source, task string) (int64, error) {
	saved, err := uc.Record(ctx, entity.TaskRun{
		Source:    source,
		Task:      task,
		Status:    entity.TaskRunRunning,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		return 0, err
	}

	return saved.ID, nil
}

// Finish closes a run opened by Started. An id of 0 means the run was never
// opened (its Record call failed), and finishing it is a no-op.
func (uc *UseCase) Finish(ctx context.Context, id int64, status, summary string, detail map[string]any, runErr error) error {
	if id == 0 {
		return nil
	}
	outcome := entity.TaskRun{Status: status, Summary: summary, Detail: detail}
	if runErr != nil {
		outcome.Error = runErr.Error()
	}
	if _, err := uc.Complete(ctx, id, outcome); err != nil {
		return err
	}

	return nil
}
