// Domain: app (stays at top-level — uses *AppStore type, not *Store)
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// LearningProfile holds the current adaptive weights for one org.
type LearningProfile struct {
	ID                 int64     `json:"id"`
	OrgID              int64     `json:"org_id"`
	KeywordRelevance   float64   `json:"keyword_relevance"`
	Engagement         float64   `json:"engagement"`
	ContentQuality     float64   `json:"content_quality"`
	ConvertedCount     int       `json:"converted_count"`
	RejectedCount      int       `json:"rejected_count"`
	IgnoredCount       int       `json:"ignored_count"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// OutcomeEvent records one feedback signal tied to a specific lead.
type OutcomeEvent struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	LeadID    int64     `json:"lead_id"`
	Category  string    `json:"category"` // hot|warm|cold
	Outcome   string    `json:"outcome"`  // converted|rejected|ignored
	Score     float64   `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

// LearningHistoryEntry is a snapshot of weights at a point in time (for charting).
type LearningHistoryEntry struct {
	ID               int64           `json:"id"`
	OrgID            int64           `json:"org_id"`
	WeightsJSON      json.RawMessage `json:"weights"`
	TriggerOutcome   string          `json:"trigger_outcome"`
	CreatedAt        time.Time       `json:"created_at"`
}

// UpsertLearningProfile creates or updates the learning profile for an org.
func (a *AppStore) UpsertLearningProfile(ctx context.Context, p LearningProfile) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO learning_profiles
			(org_id, keyword_relevance, engagement, content_quality,
			 converted_count, rejected_count, ignored_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id) DO UPDATE SET
			keyword_relevance = excluded.keyword_relevance,
			engagement        = excluded.engagement,
			content_quality   = excluded.content_quality,
			converted_count   = excluded.converted_count,
			rejected_count    = excluded.rejected_count,
			ignored_count     = excluded.ignored_count,
			updated_at        = excluded.updated_at`,
		p.OrgID, p.KeywordRelevance, p.Engagement, p.ContentQuality,
		p.ConvertedCount, p.RejectedCount, p.IgnoredCount, p.UpdatedAt.UTC(),
	)
	return err
}

func (a *AppStore) GetLearningProfile(ctx context.Context, orgID int64) (*LearningProfile, error) {
	row := a.db.QueryRowContext(ctx, `
		SELECT id, org_id, keyword_relevance, engagement, content_quality,
		       converted_count, rejected_count, ignored_count, updated_at
		FROM learning_profiles WHERE org_id = ?`, orgID)
	var p LearningProfile
	var updatedAt string
	err := row.Scan(&p.ID, &p.OrgID, &p.KeywordRelevance, &p.Engagement, &p.ContentQuality,
		&p.ConvertedCount, &p.RejectedCount, &p.IgnoredCount, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &p, nil
}

// InsertOutcomeEvent records one feedback signal.
func (a *AppStore) InsertOutcomeEvent(ctx context.Context, ev OutcomeEvent) error {
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO outcome_events (org_id, lead_id, category, outcome, score, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ev.OrgID, ev.LeadID, ev.Category, ev.Outcome, ev.Score, ev.CreatedAt.UTC(),
	)
	return err
}

// ListOutcomeEvents returns the most recent N events for an org.
func (a *AppStore) ListOutcomeEvents(ctx context.Context, orgID int64, limit int) ([]OutcomeEvent, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, org_id, lead_id, category, outcome, score, created_at
		FROM outcome_events WHERE org_id = ?
		ORDER BY created_at DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutcomeEvent
	for rows.Next() {
		var ev OutcomeEvent
		var createdAt string
		if err := rows.Scan(&ev.ID, &ev.OrgID, &ev.LeadID, &ev.Category, &ev.Outcome,
			&ev.Score, &createdAt); err != nil {
			return nil, err
		}
		ev.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// AppendLearningHistory saves a weight snapshot triggered by an outcome.
func (a *AppStore) AppendLearningHistory(ctx context.Context, orgID int64, weights any, triggerOutcome string) error {
	b, err := json.Marshal(weights)
	if err != nil {
		return err
	}
	_, err = a.db.ExecContext(ctx, `
		INSERT INTO learning_history (org_id, weights_json, trigger_outcome, created_at)
		VALUES (?, ?, ?, ?)`,
		orgID, string(b), triggerOutcome, time.Now().UTC(),
	)
	return err
}

// ListLearningHistory returns the most recent N weight snapshots for charting.
func (a *AppStore) ListLearningHistory(ctx context.Context, orgID int64, limit int) ([]LearningHistoryEntry, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, org_id, weights_json, trigger_outcome, created_at
		FROM learning_history WHERE org_id = ?
		ORDER BY created_at DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LearningHistoryEntry
	for rows.Next() {
		var e LearningHistoryEntry
		var createdAt string
		if err := rows.Scan(&e.ID, &e.OrgID, &e.WeightsJSON, &e.TriggerOutcome, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}
