package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ClassificationLogEntry records one AI classification decision — both
// kept leads AND rejected ones. Without this, "why was every post in
// my crawl rejected?" is an unanswerable question because rejected
// posts never land in leads / task_leads (ingest returns early).
type ClassificationLogEntry struct {
	ID             int64     `json:"id"`
	OrgID          int64     `json:"org_id"`
	TaskID         string    `json:"task_id"`
	AccountID      int64     `json:"account_id"`
	SourceURL      string    `json:"source_url"`
	AuthorName     string    `json:"author_name"`
	ContentSnippet string    `json:"content_snippet"`
	AIIntent       string    `json:"ai_intent"`     // potential_customer | candidate | partner | provider_ad | not_relevant | spam
	AIPriority     string    `json:"ai_priority"`   // hot | warm | cold | rejected
	AIReason       string    `json:"ai_reason"`     // one-sentence rationale from the LLM
	AIScore        float64   `json:"ai_score"`      // 0.0 - 1.0
	TargetRole     string    `json:"target_role"`   // inferred user goal (potential_customer / candidate / partner / "")
	Decision       string    `json:"decision"`      // kept | rejected | error | skipped_filter
	UserPrompt     string    `json:"user_prompt"`   // the crawl prompt for traceability
	CreatedAt      time.Time `json:"created_at"`
}

// Decision values written to classification_log.
const (
	ClassificationKept           = "kept"
	ClassificationRejected       = "rejected"
	ClassificationError          = "error"
	ClassificationSkippedFilter  = "skipped_filter"
	ClassificationCold           = "cold"
)

// RecordClassification persists one classification decision. Best-effort:
// the caller treats failure as non-fatal — losing one diagnostic row
// must never block the actual ingest pipeline.
func (s *Store) RecordClassification(ctx context.Context, e ClassificationLogEntry) error {
	if e.OrgID <= 0 {
		return fmt.Errorf("org_id is required")
	}
	if strings.TrimSpace(e.Decision) == "" {
		return fmt.Errorf("decision is required")
	}
	snippet := e.ContentSnippet
	if len(snippet) > 320 {
		snippet = snippet[:320] + "…"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO classification_log
		   (org_id, task_id, account_id, source_url, author_name, content_snippet,
		    ai_intent, ai_priority, ai_reason, ai_score, target_role, decision, user_prompt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.OrgID, e.TaskID, e.AccountID, e.SourceURL, e.AuthorName, snippet,
		e.AIIntent, e.AIPriority, e.AIReason, e.AIScore, e.TargetRole, e.Decision, e.UserPrompt,
	)
	return err
}

// ListRecentClassifications returns the most recent classification
// decisions for an org. Optional taskID narrows to a single crawl batch
// — the "show me why this crawl rejected everything" surface.
//   limit clamped to [1, 500]; 0 → 50.
func (s *Store) ListRecentClassifications(ctx context.Context, orgID int64, taskID string, decision string, limit int) ([]ClassificationLogEntry, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("org_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query := `SELECT id, org_id, task_id, account_id, source_url, author_name, content_snippet,
	                 ai_intent, ai_priority, ai_reason, ai_score, target_role, decision,
	                 user_prompt, created_at
	          FROM classification_log
	          WHERE org_id = ?`
	args := []any{orgID}
	if t := strings.TrimSpace(taskID); t != "" {
		query += ` AND task_id = ?`
		args = append(args, t)
	}
	if d := strings.TrimSpace(decision); d != "" {
		query += ` AND decision = ?`
		args = append(args, d)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ClassificationLogEntry
	for rows.Next() {
		var e ClassificationLogEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.OrgID, &e.TaskID, &e.AccountID, &e.SourceURL, &e.AuthorName,
			&e.ContentSnippet, &e.AIIntent, &e.AIPriority, &e.AIReason, &e.AIScore, &e.TargetRole,
			&e.Decision, &e.UserPrompt, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ClassificationBreakdown is the rejection-reason summary the crawl
// handler surfaces back to the user. Empty AIIntent rows are bucketed
// under "(no intent)" — happens when the LLM call errored out.
type ClassificationBreakdown struct {
	Total     int            `json:"total"`
	Kept      int            `json:"kept"`
	Rejected  int            `json:"rejected"`
	ByIntent  map[string]int `json:"by_intent"`   // intent → count of rejected
	ByReason  []ReasonCount  `json:"by_reason"`   // top-N rejection reasons
}

// ReasonCount is one bucket in the breakdown's by_reason list.
type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// SummariseClassifications builds a rejection-reason breakdown for a
// single crawl task — answers "of the N rejected, how many for each
// reason?". The crawl handler surfaces this in the completion summary.
func (s *Store) SummariseClassifications(ctx context.Context, orgID int64, taskID string) (*ClassificationBreakdown, error) {
	if orgID <= 0 || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("org_id and task_id are required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT decision, ai_intent, ai_reason
		   FROM classification_log
		  WHERE org_id = ? AND task_id = ?`,
		orgID, taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &ClassificationBreakdown{ByIntent: map[string]int{}}
	reasonHits := map[string]int{}
	for rows.Next() {
		var decision, intent, reason string
		if err := rows.Scan(&decision, &intent, &reason); err != nil {
			return nil, err
		}
		out.Total++
		switch decision {
		case ClassificationKept:
			out.Kept++
		case ClassificationRejected, ClassificationCold:
			out.Rejected++
			key := strings.TrimSpace(intent)
			if key == "" {
				key = "(no intent)"
			}
			out.ByIntent[key]++
			r := strings.TrimSpace(reason)
			if r != "" {
				reasonHits[r]++
			}
		}
	}
	// Top-10 reasons.
	type kv struct {
		k string
		v int
	}
	arr := make([]kv, 0, len(reasonHits))
	for k, v := range reasonHits {
		arr = append(arr, kv{k, v})
	}
	for i := 1; i < len(arr); i++ {
		for j := i; j > 0 && arr[j].v > arr[j-1].v; j-- {
			arr[j-1], arr[j] = arr[j], arr[j-1]
		}
	}
	if len(arr) > 10 {
		arr = arr[:10]
	}
	out.ByReason = make([]ReasonCount, len(arr))
	for i, kv := range arr {
		out.ByReason[i] = ReasonCount{Reason: kv.k, Count: kv.v}
	}
	return out, nil
}
