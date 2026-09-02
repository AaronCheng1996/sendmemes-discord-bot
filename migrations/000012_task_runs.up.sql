-- One row per execution of something worth reviewing later: a scheduled send, a
-- sync, or a crawler run reported over the ingest API.
--
-- Deliberately run-shaped rather than line-shaped. Ten log lines about one 2pm
-- push is nine too many for a dashboard, so the writer produces a single row
-- with a one-line summary and puts the steps in `detail` for whoever expands it.
-- Reconstructing runs from log lines after the fact would be far more fragile.
--
-- This is not entity.Job: that manager is in-memory, capped at 50 and lost on
-- restart, and exists to spin the UI's progress indicator. This table is the
-- durable history, and unlike Job it accepts writes from outside the process.
CREATE TABLE IF NOT EXISTS task_runs (
    id          bigserial   PRIMARY KEY,
    -- Who ran it: 'scheduled_send', 'sync', 'crawler', or whatever an ingest
    -- client calls itself. Left free-form on purpose — pinning it to a CHECK
    -- would mean a migration every time a new client starts reporting.
    source      text        NOT NULL,
    -- What it ran on: a rule name, an artist, a channel. Free-form.
    task        text        NOT NULL DEFAULT '',
    status      text        NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    started_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    summary     text        NOT NULL DEFAULT '',
    detail      jsonb       NOT NULL DEFAULT '{}'::jsonb,
    error       text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- The dashboard reads newest-first, usually filtered by source or status.
CREATE INDEX IF NOT EXISTS task_runs_started_at_idx
    ON task_runs (started_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS task_runs_source_started_at_idx
    ON task_runs (source, started_at DESC);
