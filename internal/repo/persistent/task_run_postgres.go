package persistent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/entity"
	"github.com/AaronCheng1996/sendmemes-discord-bot/internal/repo"
	"github.com/AaronCheng1996/sendmemes-discord-bot/pkg/postgres"
	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
)

// TaskRunsRepo stores one row per execution of a scheduled send, a sync, or
// anything an ingest client reports.
type TaskRunsRepo struct {
	*postgres.Postgres
}

// NewTaskRunsRepo creates a new task runs repository.
func NewTaskRunsRepo(pg *postgres.Postgres) *TaskRunsRepo {
	return &TaskRunsRepo{Postgres: pg}
}

func taskRunSelect(r *TaskRunsRepo) sq.SelectBuilder {
	return r.Builder.
		Select("id", "source", "task", "status", "started_at", "finished_at",
			"summary", "detail", "COALESCE(error, '')", "created_at").
		From("task_runs")
}

func scanTaskRun(row pgx.Row) (entity.TaskRun, error) {
	var run entity.TaskRun
	var finishedAt *time.Time
	var rawDetail []byte
	if err := row.Scan(
		&run.ID, &run.Source, &run.Task, &run.Status, &run.StartedAt, &finishedAt,
		&run.Summary, &rawDetail, &run.Error, &run.CreatedAt,
	); err != nil {
		return entity.TaskRun{}, err
	}
	run.FinishedAt = finishedAt
	if len(rawDetail) > 0 {
		if err := json.Unmarshal(rawDetail, &run.Detail); err != nil {
			return entity.TaskRun{}, fmt.Errorf("detail: %w", err)
		}
	}

	return run, nil
}

// marshalDetail renders a run's detail payload for storage. A nil map is stored
// as an empty object rather than JSON null, matching the column default.
func marshalDetail(detail map[string]any) ([]byte, error) {
	if detail == nil {
		return []byte("{}"), nil
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal detail: %w", err)
	}

	return raw, nil
}

// Insert stores a run and returns it with ID and CreatedAt filled in.
func (r *TaskRunsRepo) Insert(ctx context.Context, run entity.TaskRun) (entity.TaskRun, error) {
	detail, err := marshalDetail(run.Detail)
	if err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Insert - %w", err)
	}

	sql, args, err := r.Builder.
		Insert("task_runs").
		Columns("source", "task", "status", "started_at", "finished_at", "summary", "detail", "error").
		Values(run.Source, run.Task, run.Status, run.StartedAt, run.FinishedAt,
			run.Summary, detail, nullableString(run.Error)).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Insert - r.Builder: %w", err)
	}
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&run.ID, &run.CreatedAt); err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Insert - QueryRow: %w", err)
	}

	return run, nil
}

// Complete replaces a run's outcome fields: status, finish time, summary,
// detail and error. Everything identifying the run — source, task, start time —
// is left as it was recorded.
func (r *TaskRunsRepo) Complete(ctx context.Context, id int64, outcome entity.TaskRun) (entity.TaskRun, error) {
	detail, err := marshalDetail(outcome.Detail)
	if err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Complete - %w", err)
	}

	sql, args, err := r.Builder.
		Update("task_runs").
		Set("status", outcome.Status).
		Set("finished_at", outcome.FinishedAt).
		Set("summary", outcome.Summary).
		Set("detail", sq.Expr("?::jsonb", detail)).
		Set("error", nullableString(outcome.Error)).
		Where(sq.Eq{"id": id}).
		Suffix("RETURNING id, source, task, status, started_at, finished_at, summary, detail, COALESCE(error, ''), created_at").
		ToSql()
	if err != nil {
		return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Complete - r.Builder: %w", err)
	}

	run, err := scanTaskRun(r.Pool.QueryRow(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Complete - run %d not found", id)
		}
		return entity.TaskRun{}, fmt.Errorf("TaskRunsRepo - Complete - QueryRow: %w", err)
	}

	return run, nil
}

// applyTaskRunFilters narrows a listing to one source and/or status.
func applyTaskRunFilters(b sq.SelectBuilder, q repo.TaskRunListQuery) sq.SelectBuilder {
	if source := strings.TrimSpace(q.Source); source != "" {
		b = b.Where(sq.Eq{"source": source})
	}
	if status := strings.TrimSpace(q.Status); status != "" {
		b = b.Where(sq.Eq{"status": status})
	}

	return b
}

// List returns runs newest-first with offset/limit pagination.
func (r *TaskRunsRepo) List(ctx context.Context, q repo.TaskRunListQuery, offset, limit int) ([]entity.TaskRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	sql, args, err := applyTaskRunFilters(taskRunSelect(r), q).
		OrderBy("started_at DESC, id DESC").
		Offset(uint64(offset)).
		Limit(uint64(limit)).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("TaskRunsRepo - List - r.Builder: %w", err)
	}

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("TaskRunsRepo - List - Query: %w", err)
	}
	defer rows.Close()

	runs := make([]entity.TaskRun, 0, limit)
	for rows.Next() {
		run, scanErr := scanTaskRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("TaskRunsRepo - List - Scan: %w", scanErr)
		}
		runs = append(runs, run)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("TaskRunsRepo - List - rows.Err: %w", err)
	}

	return runs, nil
}

// Count returns how many runs match the listing filters.
func (r *TaskRunsRepo) Count(ctx context.Context, q repo.TaskRunListQuery) (int, error) {
	sql, args, err := applyTaskRunFilters(r.Builder.Select("COUNT(*)").From("task_runs"), q).ToSql()
	if err != nil {
		return 0, fmt.Errorf("TaskRunsRepo - Count - r.Builder: %w", err)
	}
	var n int
	if err = r.Pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("TaskRunsRepo - Count - QueryRow: %w", err)
	}

	return n, nil
}

// Sources returns the distinct source values present, so the dashboard can
// offer a filter without hardcoding the list of clients that report runs.
func (r *TaskRunsRepo) Sources(ctx context.Context) ([]string, error) {
	sql, args, err := r.Builder.
		Select("DISTINCT source").
		From("task_runs").
		OrderBy("source ASC").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("TaskRunsRepo - Sources - r.Builder: %w", err)
	}

	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("TaskRunsRepo - Sources - Query: %w", err)
	}
	defer rows.Close()

	sources := make([]string, 0)
	for rows.Next() {
		var s string
		if scanErr := rows.Scan(&s); scanErr != nil {
			return nil, fmt.Errorf("TaskRunsRepo - Sources - Scan: %w", scanErr)
		}
		sources = append(sources, s)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("TaskRunsRepo - Sources - rows.Err: %w", err)
	}

	return sources, nil
}

// PruneBefore deletes runs that started before cutoff and returns how many
// went. Runs are operational history, not records anyone audits, so they are
// hard-deleted — soft-deleting a retention sweep would defeat its purpose.
func (r *TaskRunsRepo) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	sql, args, err := r.Builder.
		Delete("task_runs").
		Where(sq.Lt{"started_at": cutoff}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("TaskRunsRepo - PruneBefore - r.Builder: %w", err)
	}

	tag, err := r.Pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("TaskRunsRepo - PruneBefore - Exec: %w", err)
	}

	return tag.RowsAffected(), nil
}
