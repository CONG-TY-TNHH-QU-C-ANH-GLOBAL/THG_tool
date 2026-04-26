package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the single source of truth for job state.
// MaxOpenConns(1) serialises writes; SQLite WAL mode allows concurrent readers.
type Store struct {
	db *sql.DB
}

// NewStore opens the SQLite database and runs migrations.
func NewStore(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS jobs (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id      TEXT    NOT NULL,
			intent       TEXT    NOT NULL,
			payload      TEXT    NOT NULL DEFAULT '{}',
			status       TEXT    NOT NULL DEFAULT 'pending',
			attempt      INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			error        TEXT    NOT NULL DEFAULT '',
			claimed_by   TEXT    NOT NULL DEFAULT '',
			claimed_at   DATETIME,
			progress     INTEGER NOT NULL DEFAULT 0,
			result       TEXT    NOT NULL DEFAULT '',
			created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_jobs_status_created ON jobs(status, created_at ASC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec migration: %w\nstmt: %s", err, stmt)
		}
	}
	// Idempotent column addition for existing databases that predate progress column.
	_, err := s.db.Exec(`ALTER TABLE jobs ADD COLUMN progress INTEGER NOT NULL DEFAULT 0`)
	if err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("alter jobs add progress: %w", err)
	}
	return nil
}

// Submit inserts a job with idempotency: duplicate task_id returns the existing row.
func (s *Store) Submit(ctx context.Context, task *Task, payload string) (*Job, error) {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO jobs (task_id, intent, payload, status, created_at, updated_at)
		 VALUES (?, ?, ?, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		task.TaskID, task.Intent, payload,
	)
	if err != nil {
		return nil, fmt.Errorf("insert job: %w", err)
	}
	return s.GetByTaskID(ctx, task.TaskID)
}

// Claim atomically picks the next pending job using a single UPDATE … RETURNING.
// Returns nil, nil when the queue is empty.
func (s *Store) Claim(ctx context.Context, workerID string) (*Job, error) {
	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx,
		`UPDATE jobs
		 SET status='running', claimed_by=?, claimed_at=?, updated_at=CURRENT_TIMESTAMP
		 WHERE id = (
		   SELECT id FROM jobs WHERE status='pending' ORDER BY created_at ASC LIMIT 1
		 )
		 RETURNING id, task_id, intent, payload, status, attempt, max_attempts,
		           error, claimed_by, claimed_at, progress, result, created_at, updated_at`,
		workerID, now,
	)
	j, err := scanJobRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// UpdateProgress sets the progress percentage (0–100) for a running job.
func (s *Store) UpdateProgress(ctx context.Context, id int64, progress int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET progress=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		progress, id,
	)
	return err
}

// Complete writes the JSON result and transitions the job to completed.
func (s *Store) Complete(ctx context.Context, id int64, result string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs SET status='completed', progress=100, result=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		result, id,
	)
	return err
}

// Fail increments attempt. If attempt < max_attempts the job is reset to pending for retry;
// otherwise it is marked failed.
func (s *Store) Fail(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs
		 SET status     = CASE WHEN attempt + 1 < max_attempts THEN 'pending' ELSE 'failed' END,
		     attempt    = attempt + 1,
		     error      = ?,
		     claimed_by = '',
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		errMsg, id,
	)
	return err
}

// RecoverStale resets jobs stuck in running state for longer than timeout back to pending.
func (s *Store) RecoverStale(ctx context.Context, timeout time.Duration) error {
	cutoff := time.Now().UTC().Add(-timeout)
	_, err := s.db.ExecContext(ctx,
		`UPDATE jobs
		 SET status='pending', claimed_by='', claimed_at=NULL, updated_at=CURRENT_TIMESTAMP
		 WHERE status='running' AND claimed_at < ?`,
		cutoff,
	)
	return err
}

// GetByTaskID fetches a job by its idempotency key.
func (s *Store) GetByTaskID(ctx context.Context, taskID string) (*Job, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, task_id, intent, payload, status, attempt, max_attempts,
		        error, claimed_by, claimed_at, progress, result, created_at, updated_at
		 FROM jobs WHERE task_id = ?`, taskID,
	)
	j, err := scanJobRow(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("job not found: %s", taskID)
	}
	return j, err
}

// List returns up to limit jobs, optionally filtered by status.
func (s *Store) List(ctx context.Context, status string, limit int) ([]Job, error) {
	q := `SELECT id, task_id, intent, payload, status, attempt, max_attempts,
		         error, claimed_by, claimed_at, progress, result, created_at, updated_at
		  FROM jobs`
	args := []any{}
	if status != "" {
		q += " WHERE status = ?"
		args = append(args, status)
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		j, err := scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// ── scan helpers ──────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scan(r scanner) (*Job, error) {
	var j Job
	var claimedAt sql.NullTime
	err := r.Scan(
		&j.ID, &j.TaskID, &j.Intent, &j.Payload, &j.Status,
		&j.Attempt, &j.MaxAttempts, &j.Error, &j.ClaimedBy, &claimedAt,
		&j.Progress, &j.Result, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if claimedAt.Valid {
		j.ClaimedAt = &claimedAt.Time
	}
	return &j, nil
}

func scanJobRow(r *sql.Row) (*Job, error)  { return scan(r) }
func scanJobRows(r *sql.Rows) (*Job, error) { return scan(r) }

// StatusCounts is a summary of job counts grouped by status.
type StatusCounts struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// GetStatusCounts returns the count of jobs in each status bucket.
func (s *Store) GetStatusCounts(ctx context.Context) (StatusCounts, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return StatusCounts{}, err
	}
	defer rows.Close()

	var c StatusCounts
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			continue
		}
		switch status {
		case StatusPending:
			c.Pending = n
		case StatusRunning:
			c.Running = n
		case StatusCompleted:
			c.Completed = n
		case StatusFailed:
			c.Failed = n
		}
	}
	return c, rows.Err()
}
