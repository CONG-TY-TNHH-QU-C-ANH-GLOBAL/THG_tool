package store

import (
	"database/sql"

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
func (s *Store) GetNextLocalJob() (*models.Job, error) {
	row := s.db.QueryRow(`SELECT id, type, platform, target, status, COALESCE(execution_mode,'local'), COALESCE(result,''), COALESCE(error,''), created_at, COALESCE(started_at,created_at), COALESCE(done_at,created_at) FROM jobs WHERE status = 'pending' AND execution_mode = 'local' ORDER BY created_at ASC LIMIT 1`)
	var j models.Job
	err := row.Scan(&j.ID, &j.Type, &j.Platform, &j.Target, &j.Status, &j.ExecutionMode, &j.Result, &j.Error, &j.CreatedAt, &j.StartedAt, &j.DoneAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
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
