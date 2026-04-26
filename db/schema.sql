-- Orchestrator job queue schema
-- Applied automatically by jobs.NewStore(); this file is the reference copy.

PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS jobs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id      TEXT    NOT NULL,
    intent       TEXT    NOT NULL,
    payload      TEXT    NOT NULL DEFAULT '{}',
    status       TEXT    NOT NULL DEFAULT 'pending',  -- pending|running|completed|failed
    attempt      INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    error        TEXT    NOT NULL DEFAULT '',
    claimed_by   TEXT    NOT NULL DEFAULT '',
    claimed_at   DATETIME,
    result       TEXT    NOT NULL DEFAULT '',          -- JSON OutputDataset on completion
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(task_id)
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_created
    ON jobs(status, created_at ASC);
