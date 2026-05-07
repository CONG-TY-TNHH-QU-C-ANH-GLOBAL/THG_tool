package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// UpdateLeadClassification overwrites the AI-derived fields on an existing
// lead row. Used by the reclassify endpoint to retag legacy leads when the
// classifier prompt or business profile changes — without losing the
// original crawl payload (author, content, source_url, etc.).
func (s *Store) UpdateLeadClassification(orgID, leadID int64, score, serviceMatch, authorRole, painPoint, aiReasoning string) error {
	_, err := s.db.Exec(
		`UPDATE leads
		 SET score = ?, service_match = ?, author_role = ?, pain_point = ?, ai_reasoning = ?,
		     classified_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND COALESCE(org_id, 0) = ?`,
		score, serviceMatch, authorRole, painPoint, aiReasoning, leadID, orgID,
	)
	return err
}

// GetLeadsForReclassify returns leads owned by the org that match the
// reclassify scope. When onlyUnknown is true, the query filters to leads
// whose author_role is empty/null/"unknown" — leaving manually labelled
// leads alone. limit is clamped to a safe ceiling by the caller.
func (s *Store) GetLeadsForReclassify(orgID int64, onlyUnknown bool, limit int) ([]models.Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT l.id, COALESCE(l.org_id,0), l.source_type, l.source_id,
	           COALESCE(NULLIF(l.source_url, ''), p.url, '') as source_url,
	           l.platform, l.author, l.author_url, l.content, l.score, l.service_match,
	           l.author_role, l.pain_point, l.ai_reasoning, COALESCE(NULLIF(l.niche,''),'logistics'),
	           l.classified_at, l.created_at
	          FROM leads l LEFT JOIN posts p ON l.source_id = p.id
	          LEFT JOIN groups g ON p.group_id = g.id
	          WHERE (COALESCE(NULLIF(l.org_id,0), g.org_id, 0) = ?)`
	args := []any{orgID}
	if onlyUnknown {
		query += ` AND (l.author_role IS NULL OR TRIM(l.author_role) = '' OR LOWER(l.author_role) = 'unknown')`
	}
	query += ` ORDER BY l.created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []models.Lead
	for rows.Next() {
		var l models.Lead
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SourceType, &l.SourceID, &l.SourceURL, &l.Platform,
			&l.Author, &l.AuthorURL, &l.Content, &l.Score, &l.ServiceMatch,
			&l.AuthorRole, &l.PainPoint, &l.AIReasoning, &l.Niche,
			&l.ClassifiedAt, &l.CreatedAt); err != nil {
			return nil, err
		}
		leads = append(leads, l)
	}
	return leads, nil
}

// InsertLead inserts a classified lead.
func (s *Store) InsertLead(l *models.Lead) (int64, error) {
	if l.Niche == "" {
		l.Niche = "logistics"
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO leads (org_id, source_type, source_id, source_url, platform, author, author_url, content, score, service_match, author_role, pain_point, ai_reasoning, niche, classified_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.OrgID, l.SourceType, l.SourceID, l.SourceURL, l.Platform, l.Author, l.AuthorURL, l.Content,
		l.Score, l.ServiceMatch, l.AuthorRole, l.PainPoint, l.AIReasoning, l.Niche, l.ClassifiedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLeads returns leads with optional filtering by score.
func (s *Store) GetLeads(score string, limit, offset int) ([]models.Lead, error) {
	return s.GetLeadsFiltered(score, "", limit, offset, 0)
}

// GetLeadsFiltered returns leads filtered by score, niche, and org. orgID=0 returns all.
func (s *Store) GetLeadsFiltered(score, niche string, limit, offset int, orgID int64) ([]models.Lead, error) {
	query := `SELECT l.id, COALESCE(l.org_id,0), l.source_type, l.source_id,
	           COALESCE(NULLIF(l.source_url, ''), p.url, '') as source_url,
	           l.platform, l.author, l.author_url, l.content, l.score, l.service_match,
	           l.author_role, l.pain_point, l.ai_reasoning, COALESCE(NULLIF(l.niche,''),'logistics'),
	           l.classified_at, l.created_at,
	           EXISTS(SELECT 1 FROM outbound_messages om WHERE om.target_url = COALESCE(NULLIF(l.source_url,''),p.url,'') AND om.type='comment' AND om.status = 'sent') as commented
	          FROM leads l LEFT JOIN posts p ON l.source_id = p.id`
	if orgID > 0 {
		query += ` LEFT JOIN groups g ON p.group_id = g.id`
	}

	var args []any
	var where []string
	if orgID > 0 {
		where = append(where, "(COALESCE(NULLIF(l.org_id,0), g.org_id, 0) = ?)")
		args = append(args, orgID)
	}
	if score != "" {
		where = append(where, "l.score = ?")
		args = append(args, score)
	}
	if niche != "" {
		where = append(where, "COALESCE(NULLIF(l.niche,''),'logistics') = ?")
		args = append(args, niche)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY l.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []models.Lead
	for rows.Next() {
		var l models.Lead
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SourceType, &l.SourceID, &l.SourceURL, &l.Platform,
			&l.Author, &l.AuthorURL, &l.Content, &l.Score, &l.ServiceMatch,
			&l.AuthorRole, &l.PainPoint, &l.AIReasoning, &l.Niche,
			&l.ClassifiedAt, &l.CreatedAt, &l.Commented); err != nil {
			return nil, err
		}
		leads = append(leads, l)
	}
	return leads, nil
}

// GetAutomationLeadsForOrg returns leads that automation can act on.
func (s *Store) GetAutomationLeadsForOrg(orgID int64, score string, limit int) ([]models.Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	legacy, err := s.GetLeadsFiltered(normalizeLeadScoreFilter(score), "", limit, 0, orgID)
	if err != nil {
		return nil, err
	}

	out := make([]models.Lead, 0, limit)
	seen := map[string]bool{}
	add := func(l models.Lead) {
		if len(out) >= limit {
			return
		}
		key := strings.TrimSpace(l.AuthorURL)
		if key == "" {
			key = strings.TrimSpace(l.SourceURL)
		}
		if key == "" {
			key = fmt.Sprintf("lead:%d", l.ID)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, l)
	}
	for _, l := range legacy {
		add(l)
	}
	if len(out) >= limit {
		return out, nil
	}

	if taskLeads, err := s.getTaskLeadsForAutomation(orgID, score, limit-len(out)); err == nil {
		for _, l := range taskLeads {
			add(l)
		}
	} else if !strings.Contains(err.Error(), "no such table") {
		return out, err
	}
	return out, nil
}

// parseAIIntentAndReason extracts ai_intent and ai_reason values from a
// task_leads.signals_json blob. The ingest pipeline appends entries like
// "ai_intent:candidate" and "ai_reason:Author is hiring sales POD staff"
// alongside other signals. This helper unpacks them so the dashboard layer
// can render the intent tag + reason without re-running the classifier.
func parseAIIntentAndReason(signalsJSON string) (intent, reason string) {
	var signals []string
	if err := json.Unmarshal([]byte(signalsJSON), &signals); err != nil {
		return "", ""
	}
	for _, s := range signals {
		if rest, ok := strings.CutPrefix(s, "ai_intent:"); ok && intent == "" {
			intent = strings.TrimSpace(rest)
		} else if rest, ok := strings.CutPrefix(s, "ai_reason:"); ok && reason == "" {
			reason = strings.TrimSpace(rest)
		}
	}
	return intent, reason
}

func normalizeLeadScoreFilter(score string) string {
	score = strings.ToLower(strings.TrimSpace(score))
	switch score {
	case "", "all":
		return ""
	case "hot", "warm", "cold":
		return score
	default:
		return ""
	}
}

func (s *Store) getTaskLeadsForAutomation(orgID int64, score string, limit int) ([]models.Lead, error) {
	query := `SELECT id, org_id, source_url, author_profile_url, author_name, content, lead_score, category, signals_json, created_at
		FROM task_leads WHERE org_id = ?`
	args := []any{orgID}
	if f := normalizeLeadScoreFilter(score); f != "" {
		query += ` AND category = ?`
		args = append(args, f)
	}
	query += ` ORDER BY lead_score DESC, created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leads []models.Lead
	for rows.Next() {
		var l models.Lead
		var numericScore float64
		var signalsJSON string
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SourceURL, &l.AuthorURL, &l.Author, &l.Content, &numericScore, &l.Score, &signalsJSON, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.SourceType = "task_lead"
		l.Platform = models.PlatformFacebook
		l.ServiceMatch = string(l.Score)
		// Recover ai_intent and ai_reason from signals_json so the dashboard
		// can show the same intent tag + reason for task_leads as for the
		// legacy mirror. The ingest pipeline writes both into signals.
		intent, reason := parseAIIntentAndReason(signalsJSON)
		if intent == "" {
			intent = "unknown"
		}
		l.AuthorRole = intent
		if reason != "" {
			l.PainPoint = reason
		} else {
			l.PainPoint = fmt.Sprintf("score %.0f", numericScore)
		}
		l.ClassifiedAt = l.CreatedAt
		leads = append(leads, l)
	}
	return leads, rows.Err()
}

// DeleteLead removes a lead by ID from the legacy leads table AND any matching
// task_leads row (by source_url + org_id) so the dashboard does not re-render
// it on refresh. The ingest pipeline mirrors task_leads → leads, so deleting
// only one side leaves a ghost copy that re-appears on the next list call.
func (s *Store) DeleteLead(leadID int64) error {
	var (
		orgID     int64
		sourceURL string
	)
	row := s.db.QueryRow(`SELECT org_id, COALESCE(source_url, '') FROM leads WHERE id = ?`, leadID)
	switch err := row.Scan(&orgID, &sourceURL); err {
	case nil:
		// Best effort: delete the corresponding task_leads row too.
		if strings.TrimSpace(sourceURL) != "" && orgID > 0 {
			_, _ = s.db.Exec(`DELETE FROM task_leads WHERE org_id = ? AND source_url = ?`, orgID, sourceURL)
		}
	case sql.ErrNoRows:
		// Not in legacy leads — fall through and try task_leads by id.
	default:
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM leads WHERE id = ?`, leadID); err != nil {
		return err
	}
	return nil
}

// DeleteTaskLead removes a task_leads row by id (scoped to org for safety) and
// best-effort deletes the legacy leads mirror by matching source_url.
func (s *Store) DeleteTaskLead(orgID, leadID int64) error {
	var sourceURL string
	row := s.db.QueryRow(`SELECT COALESCE(source_url, '') FROM task_leads WHERE id = ? AND org_id = ?`, leadID, orgID)
	if err := row.Scan(&sourceURL); err == nil && strings.TrimSpace(sourceURL) != "" {
		_, _ = s.db.Exec(`DELETE FROM leads WHERE org_id = ? AND source_url = ?`, orgID, sourceURL)
	} else if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM task_leads WHERE id = ? AND org_id = ?`, leadID, orgID); err != nil {
		return err
	}
	return nil
}

// DeleteLeads removes leads scoped by niche. Empty niche deletes all leads.
func (s *Store) DeleteLeads(niche string) (int64, error) {
	var result sql.Result
	var err error
	if niche == "" {
		result, err = s.db.Exec(`DELETE FROM leads`)
	} else {
		result, err = s.db.Exec(`DELETE FROM leads WHERE niche = ?`, niche)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
