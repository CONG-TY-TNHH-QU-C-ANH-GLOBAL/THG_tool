package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
)

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
	query := `SELECT id, org_id, source_url, author_profile_url, author_name, content, lead_score, category, created_at
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
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SourceURL, &l.AuthorURL, &l.Author, &l.Content, &numericScore, &l.Score, &l.CreatedAt); err != nil {
			return nil, err
		}
		l.SourceType = "task_lead"
		l.Platform = models.PlatformFacebook
		l.ServiceMatch = string(l.Score)
		l.AuthorRole = "AI classifier"
		l.PainPoint = fmt.Sprintf("score %.0f", numericScore)
		l.ClassifiedAt = l.CreatedAt
		leads = append(leads, l)
	}
	return leads, rows.Err()
}

// DeleteLead removes a lead by ID.
func (s *Store) DeleteLead(leadID int64) error {
	_, err := s.db.Exec(`DELETE FROM leads WHERE id = ?`, leadID)
	return err
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
