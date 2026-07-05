// Domain: app (see internal/store/DOMAINS.md)
package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// app_tasks CRUD — moved verbatim from the retired *AppStore
// (AppStore dissolution PR6, 2026-07-05). ListTasks was dropped in the
// move: zero callers repo-wide (grep + build proof in the PR).

// AppTask is the application-level task record (distinct from the jobs queue row).
type AppTask struct {
	ID            int64     `json:"id"`
	TaskID        string    `json:"task_id"`
	OrgID         int64     `json:"org_id"`
	Intent        string    `json:"intent"`
	Status        string    `json:"status"`
	TotalFetched  int       `json:"total_fetched"`
	TotalReturned int       `json:"total_returned"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *Store) CreateTask(ctx context.Context, taskID string, orgID int64, intent string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO app_tasks (task_id, org_id, intent) VALUES (?, ?, ?)`,
		taskID, orgID, intent,
	)
	return err
}

func (s *Store) StartTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_tasks SET status='running', updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, taskID)
	return err
}

func (s *Store) CompleteTask(ctx context.Context, taskID string, fetched, returned int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_tasks
		 SET status='completed', total_fetched=?, total_returned=?, updated_at=CURRENT_TIMESTAMP
		 WHERE task_id=?`,
		fetched, returned, taskID,
	)
	return err
}

func (s *Store) FailTask(ctx context.Context, taskID, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE app_tasks SET status='failed', error=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		errMsg, taskID,
	)
	return err
}

func (s *Store) GetTask(ctx context.Context, taskID string) (*AppTask, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, task_id, org_id, intent, status, total_fetched, total_returned, error, created_at, updated_at
		 FROM app_tasks WHERE task_id=?`, taskID,
	)
	var t AppTask
	err := row.Scan(&t.ID, &t.TaskID, &t.OrgID, &t.Intent, &t.Status,
		&t.TotalFetched, &t.TotalReturned, &t.Error, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return &t, err
}
