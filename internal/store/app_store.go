package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AppStore manages multi-tenant task execution records and task-derived leads.
// It reuses the existing Store's DB connection; call NewAppStore(existing *Store).
// Tables are prefixed app_ / task_ to avoid collision with the legacy schema.
type AppStore struct {
	db *sql.DB
}

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

// TaskLead is a scored prospect produced by the crawl handler.
type TaskLead struct {
	ID               int64     `json:"id"`
	TaskID           string    `json:"task_id"`
	OrgID            int64     `json:"org_id"`
	SourceURL        string    `json:"source_url"`
	AuthorProfileURL string    `json:"author_profile_url"`
	AuthorName       string    `json:"author_name"`
	Content          string    `json:"content"`
	LeadScore        float64   `json:"lead_score"`
	Category         string    `json:"category"` // hot | warm | cold
	Signals          []string  `json:"signals"`
	CreatedAt        time.Time `json:"created_at"`
}

// NewAppStore wraps the existing Store's database connection,
// ensuring the app_tasks and task_leads tables exist.
func NewAppStore(s *Store) (*AppStore, error) {
	a := &AppStore{db: s.db}
	if err := a.migrate(); err != nil {
		return nil, fmt.Errorf("app store migrate: %w", err)
	}
	return a, nil
}

func (a *AppStore) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_tasks (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id        TEXT    NOT NULL UNIQUE,
			org_id         INTEGER NOT NULL DEFAULT 0,
			intent         TEXT    NOT NULL,
			status         TEXT    NOT NULL DEFAULT 'pending',
			total_fetched  INTEGER NOT NULL DEFAULT 0,
			total_returned INTEGER NOT NULL DEFAULT 0,
			error          TEXT    NOT NULL DEFAULT '',
			created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_app_tasks_org ON app_tasks(org_id, intent, status, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS task_leads (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id             TEXT    NOT NULL,
			org_id              INTEGER NOT NULL DEFAULT 0,
			source_url          TEXT    NOT NULL,
			author_profile_url  TEXT    NOT NULL DEFAULT '',
			author_name         TEXT    NOT NULL DEFAULT '',
			content             TEXT    NOT NULL DEFAULT '',
			lead_score          REAL    NOT NULL DEFAULT 0,
			category            TEXT    NOT NULL DEFAULT 'cold',
			signals_json        TEXT    NOT NULL DEFAULT '[]',
			created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(task_id, source_url)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_task_leads_org ON task_leads(org_id, category, lead_score DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := a.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec: %w\nstmt: %s", err, stmt)
		}
	}
	return nil
}

// ── AppTask CRUD ───────────────────────────────────────────────────────────────

func (a *AppStore) CreateTask(ctx context.Context, taskID string, orgID int64, intent string) error {
	_, err := a.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO app_tasks (task_id, org_id, intent) VALUES (?, ?, ?)`,
		taskID, orgID, intent,
	)
	return err
}

func (a *AppStore) StartTask(ctx context.Context, taskID string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE app_tasks SET status='running', updated_at=CURRENT_TIMESTAMP WHERE task_id=?`, taskID)
	return err
}

func (a *AppStore) CompleteTask(ctx context.Context, taskID string, fetched, returned int) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE app_tasks
		 SET status='completed', total_fetched=?, total_returned=?, updated_at=CURRENT_TIMESTAMP
		 WHERE task_id=?`,
		fetched, returned, taskID,
	)
	return err
}

func (a *AppStore) FailTask(ctx context.Context, taskID, errMsg string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE app_tasks SET status='failed', error=?, updated_at=CURRENT_TIMESTAMP WHERE task_id=?`,
		errMsg, taskID,
	)
	return err
}

func (a *AppStore) GetTask(ctx context.Context, taskID string) (*AppTask, error) {
	row := a.db.QueryRowContext(ctx,
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

func (a *AppStore) ListTasks(ctx context.Context, orgID int64, intent, status string, limit, offset int) ([]AppTask, error) {
	q := `SELECT id, task_id, org_id, intent, status, total_fetched, total_returned, error, created_at, updated_at
	      FROM app_tasks WHERE org_id=?`
	args := []any{orgID}
	if intent != "" {
		q += " AND intent=?"
		args = append(args, intent)
	}
	if status != "" {
		q += " AND status=?"
		args = append(args, status)
	}
	q += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AppTask
	for rows.Next() {
		var t AppTask
		if err := rows.Scan(&t.ID, &t.TaskID, &t.OrgID, &t.Intent, &t.Status,
			&t.TotalFetched, &t.TotalReturned, &t.Error, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── TaskLead CRUD ─────────────────────────────────────────────────────────────

func (a *AppStore) InsertLead(ctx context.Context, taskID string, orgID int64, lead TaskLead) error {
	sigJSON, _ := json.Marshal(lead.Signals)
	_, err := a.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO task_leads
		 (task_id, org_id, source_url, author_profile_url, author_name, content, lead_score, category, signals_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, orgID, lead.SourceURL, lead.AuthorProfileURL, lead.AuthorName,
		lead.Content, lead.LeadScore, lead.Category, string(sigJSON),
	)
	return err
}

func (a *AppStore) ListLeads(ctx context.Context, orgID int64, category, keyword string, minScore float64, limit, offset int) ([]TaskLead, error) {
	q := `SELECT id, task_id, org_id, source_url, author_profile_url, author_name, content,
	             lead_score, category, signals_json, created_at
	      FROM task_leads WHERE org_id=? AND lead_score >= ?`
	args := []any{orgID, minScore}
	if category != "" {
		q += " AND category=?"
		args = append(args, category)
	}
	if keyword != "" {
		q += " AND (content LIKE ? OR author_name LIKE ?)"
		args = append(args, "%"+keyword+"%", "%"+keyword+"%")
	}
	q += " ORDER BY lead_score DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaskLead
	for rows.Next() {
		var l TaskLead
		var sigJSON string
		if err := rows.Scan(&l.ID, &l.TaskID, &l.OrgID, &l.SourceURL, &l.AuthorProfileURL,
			&l.AuthorName, &l.Content, &l.LeadScore, &l.Category, &sigJSON, &l.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(sigJSON), &l.Signals)
		out = append(out, l)
	}
	return out, rows.Err()
}

// ── aggregate queries ─────────────────────────────────────────────────────────

// LeadCounts summarises task_leads grouped by category for a given org.
type LeadCounts struct {
	Total int `json:"total"`
	Hot   int `json:"hot"`
	Warm  int `json:"warm"`
	Cold  int `json:"cold"`
}

func (a *AppStore) GetLeadCounts(ctx context.Context, orgID int64) (LeadCounts, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT category, COUNT(*) FROM task_leads WHERE org_id=? GROUP BY category`, orgID)
	if err != nil {
		return LeadCounts{}, err
	}
	defer rows.Close()

	var c LeadCounts
	for rows.Next() {
		var cat string
		var n int
		if err := rows.Scan(&cat, &n); err != nil {
			continue
		}
		c.Total += n
		switch cat {
		case "hot":
			c.Hot = n
		case "warm":
			c.Warm = n
		case "cold":
			c.Cold = n
		}
	}
	return c, rows.Err()
}

// ListLeadsSince fetches leads with id > lastID ordered by id ASC (for SSE polling).
func (a *AppStore) ListLeadsSince(ctx context.Context, lastID int64, limit int) ([]TaskLead, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT id, task_id, org_id, source_url, author_profile_url, author_name, content,
		        lead_score, category, signals_json, created_at
		 FROM task_leads WHERE id > ? ORDER BY id ASC LIMIT ?`, lastID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaskLead
	for rows.Next() {
		var l TaskLead
		var sigJSON string
		if err := rows.Scan(&l.ID, &l.TaskID, &l.OrgID, &l.SourceURL, &l.AuthorProfileURL,
			&l.AuthorName, &l.Content, &l.LeadScore, &l.Category, &sigJSON, &l.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(sigJSON), &l.Signals)
		out = append(out, l)
	}
	return out, rows.Err()
}
