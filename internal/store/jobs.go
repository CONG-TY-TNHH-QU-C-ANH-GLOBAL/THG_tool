package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/models"
)

// CreateJob creates a new job entry.
func (s *Store) CreateJob(j *models.Job) (int64, error) {
	if j.ExecutionMode == "" {
		j.ExecutionMode = models.ExecutionServer
	}
	res, err := s.db.Exec(
		`INSERT INTO jobs (type, platform, target, status, execution_mode) VALUES (?, ?, ?, ?, ?)`,
		j.Type, j.Platform, j.Target, models.JobPending, j.ExecutionMode,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateJobStatus updates a job's status and optional result/error.
func (s *Store) UpdateJobStatus(jobID int64, status models.JobStatus, result, errMsg string) error {
	switch status {
	case models.JobRunning:
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, started_at = CURRENT_TIMESTAMP WHERE id = ?`, status, jobID)
		return err
	case models.JobDone, models.JobFailed:
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, result = ?, error = ?, done_at = CURRENT_TIMESTAMP WHERE id = ?`, status, result, errMsg, jobID)
		return err
	default:
		_, err := s.db.Exec(`UPDATE jobs SET status = ? WHERE id = ?`, status, jobID)
		return err
	}
}

// GetNextLocalJob returns the oldest pending job with execution_mode='local'.
//
// DEPRECATED in Phase 2.3 — this is a non-atomic peek. Two agents both
// calling this and then UpdateJobStatus(JobRunning) can race and both
// pick up the same job. Use ClaimNextLocalJob(workerID) instead, which
// performs the SELECT and UPDATE in one statement and stamps claimed_by.
func (s *Store) GetNextLocalJob() (*models.Job, error) {
	row := s.db.QueryRow(`SELECT id, type, platform, target, status, COALESCE(execution_mode,'local'), COALESCE(result,''), COALESCE(error,''), created_at, COALESCE(started_at,created_at), COALESCE(done_at,created_at) FROM jobs WHERE status = 'pending' AND execution_mode = 'local' ORDER BY created_at ASC LIMIT 1`)
	var j models.Job
	err := row.Scan(&j.ID, &j.Type, &j.Platform, &j.Target, &j.Status, &j.ExecutionMode, &j.Result, &j.Error, &j.CreatedAt, &j.StartedAt, &j.DoneAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

// ClaimNextLocalJob atomically picks the oldest pending local-runtime job
// and transitions it to running, stamping claimed_by + claimed_at +
// started_at. Two agents calling this concurrently will only ever see one
// of them receive the job; the other gets (nil, nil).
//
// workerID should be a stable identifier for the agent (e.g. agent token
// fingerprint) so RecoverStaleLocalJobs can recognise abandoned claims.
//
// Implementation note: the UPDATE writes both `status='running'` and the
// claim metadata in a single statement so the (SELECT … LIMIT 1) inside
// the WHERE clause is the locking decision — two callers cannot both
// match the same id. We then fetch the full Job row by id rather than
// relying on RETURNING, because the modernc.org/sqlite driver does not
// coerce CURRENT_TIMESTAMP results from RETURNING into time.Time the way
// it does for ordinary SELECTs.
func (s *Store) ClaimNextLocalJob(workerID string) (*models.Job, error) {
	res, err := s.db.Exec(
		`UPDATE jobs
		 SET status = 'running',
		     started_at = CURRENT_TIMESTAMP,
		     claimed_by = ?,
		     claimed_at = CURRENT_TIMESTAMP
		 WHERE id = (
		   SELECT id FROM jobs
		   WHERE status = 'pending' AND execution_mode = 'local'
		   ORDER BY created_at ASC LIMIT 1
		 )`,
		workerID,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, nil
	}
	// Fetch the row we just claimed for this worker. claimed_by is a
	// strong enough key — only one row in the jobs table is currently
	// running with this worker fingerprint stamped CURRENT_TIMESTAMP.
	//
	// Scan time columns through sql.NullString and parse manually because
	// modernc.org/sqlite does not coerce values that flow through COALESCE
	// (or RETURNING) into time.Time — the column type metadata is lost.
	row := s.db.QueryRow(
		`SELECT id, type, platform, target, status,
		        COALESCE(execution_mode,'local'),
		        COALESCE(result,''), COALESCE(error,''),
		        created_at, COALESCE(started_at, created_at),
		        COALESCE(done_at, created_at)
		 FROM jobs
		 WHERE status = 'running' AND claimed_by = ?
		 ORDER BY claimed_at DESC LIMIT 1`,
		workerID,
	)
	var j models.Job
	var createdAt, startedAt, doneAt sql.NullString
	if err := row.Scan(&j.ID, &j.Type, &j.Platform, &j.Target, &j.Status, &j.ExecutionMode, &j.Result, &j.Error, &createdAt, &startedAt, &doneAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	j.CreatedAt = parseSQLiteTime(createdAt.String)
	j.StartedAt = parseSQLiteTime(startedAt.String)
	j.DoneAt = parseSQLiteTime(doneAt.String)
	return &j, nil
}

// RecoverStaleLocalJobs resets jobs stuck in running for longer than
// timeout back to pending so another agent can pick them up. Call this
// from a background loop with a generous timeout (e.g. 10 minutes) so
// genuine slow jobs aren't preempted.
func (s *Store) RecoverStaleLocalJobs(timeout time.Duration) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE jobs
		 SET status = 'pending', claimed_by = '', claimed_at = NULL
		 WHERE execution_mode = 'local'
		   AND status = 'running'
		   AND claimed_at IS NOT NULL
		   AND claimed_at < datetime('now', ?)`,
		fmt.Sprintf("-%d seconds", int(timeout.Seconds())),
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetJobs returns jobs filtered by status.
func (s *Store) GetJobs(status string, limit int) ([]models.Job, error) {
	query := `SELECT id, type, platform, target, status, COALESCE(execution_mode,'server'), COALESCE(result,''), COALESCE(error,''), created_at, COALESCE(started_at, created_at), COALESCE(done_at, created_at) FROM jobs`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		var j models.Job
		if err := rows.Scan(&j.ID, &j.Type, &j.Platform, &j.Target, &j.Status, &j.ExecutionMode, &j.Result, &j.Error, &j.CreatedAt, &j.StartedAt, &j.DoneAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
