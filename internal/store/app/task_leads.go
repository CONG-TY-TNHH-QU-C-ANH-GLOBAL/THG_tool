// Domain: app (see internal/store/DOMAINS.md)
package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// task_leads CRUD — moved verbatim from the retired *AppStore
// (AppStore dissolution PR6, 2026-07-05).

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
	Category         string    `json:"category"`    // hot | warm | cold
	ThreadRole       string    `json:"thread_role"` // intent_originator | supplier_responder | ... (Phase B)
	Signals          []string  `json:"signals"`
	CreatedAt        time.Time `json:"created_at"`
}

func (s *Store) InsertLead(ctx context.Context, taskID string, orgID int64, lead TaskLead) error {
	sigJSON, _ := json.Marshal(lead.Signals)
	threadRole := strings.TrimSpace(lead.ThreadRole)
	if threadRole == "" {
		threadRole = "intent_originator"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO task_leads
		 (task_id, org_id, source_url, author_profile_url, author_name, content, lead_score, category, thread_role, signals_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, orgID, lead.SourceURL, lead.AuthorProfileURL, lead.AuthorName,
		lead.Content, lead.LeadScore, lead.Category, threadRole, string(sigJSON),
	)
	return err
}

func (s *Store) HasLeadWithSourceURL(ctx context.Context, orgID int64, sourceURL string) (bool, error) {
	if sourceURL == "" {
		return false, nil
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM task_leads WHERE org_id = ? AND source_url = ? LIMIT 1`, orgID, sourceURL).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ListLeads(ctx context.Context, orgID int64, category, keyword string, minScore float64, limit, offset int) ([]TaskLead, error) {
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

	rows, err := s.db.QueryContext(ctx, q, args...)
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

// LeadCounts summarises task_leads grouped by category for a given org.
type LeadCounts struct {
	Total int `json:"total"`
	Hot   int `json:"hot"`
	Warm  int `json:"warm"`
	Cold  int `json:"cold"`
}

func (s *Store) GetLeadCounts(ctx context.Context, orgID int64) (LeadCounts, error) {
	rows, err := s.db.QueryContext(ctx,
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
