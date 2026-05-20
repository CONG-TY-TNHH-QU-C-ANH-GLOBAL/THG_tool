package crawl

import "github.com/thg/scraper/internal/models"

// UpsertGroupQuality inserts or replaces the quality record for a group.
func (s *Store) UpsertGroupQuality(q *models.GroupQuality) error {
	_, err := s.db.Exec(`
		INSERT INTO group_quality
			(group_id, category, relevance_score, professionalism_score, content_quality_score,
			 spam_penalty, final_score, decision, reason, whitelist, blacklist, scored_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(group_id) DO UPDATE SET
			category=excluded.category, relevance_score=excluded.relevance_score,
			professionalism_score=excluded.professionalism_score,
			content_quality_score=excluded.content_quality_score,
			spam_penalty=excluded.spam_penalty, final_score=excluded.final_score,
			decision=excluded.decision, reason=excluded.reason,
			whitelist=excluded.whitelist, blacklist=excluded.blacklist,
			scored_at=CURRENT_TIMESTAMP`,
		q.GroupID, q.Category, q.RelevanceScore, q.ProfessionalismScore, q.ContentQualityScore,
		q.SpamPenalty, q.FinalScore, q.Decision, q.Reason, q.Whitelist, q.Blacklist,
	)
	return err
}

// GetGroupQuality returns the quality record for a group, if it
// exists. JOIN is intra-domain (group_quality ↔ groups — both crawl-
// owned), so no cross-domain projection annotation required.
func (s *Store) GetGroupQuality(groupID int64) (*models.GroupQuality, error) {
	var q models.GroupQuality
	var scoredAt, lastPostAt string
	err := s.db.QueryRow(`
		SELECT gq.group_id, g.name, g.url, gq.category, gq.relevance_score,
		       gq.professionalism_score, gq.content_quality_score, gq.spam_penalty,
		       gq.final_score, gq.decision, gq.reason, gq.whitelist, gq.blacklist,
		       COALESCE(gq.scored_at,''), COALESCE(gq.last_post_at,''),
		       gq.weekly_post_count, gq.candidate_yield, gq.spam_yield
		FROM group_quality gq JOIN groups g ON g.id = gq.group_id
		WHERE gq.group_id = ?`, groupID,
	).Scan(&q.GroupID, &q.GroupName, &q.GroupURL, &q.Category,
		&q.RelevanceScore, &q.ProfessionalismScore, &q.ContentQualityScore, &q.SpamPenalty,
		&q.FinalScore, &q.Decision, &q.Reason, &q.Whitelist, &q.Blacklist,
		&scoredAt, &lastPostAt, &q.WeeklyPostCount, &q.CandidateYield, &q.SpamYield)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// GetQualityGroupsForDomain returns usable groups for a job domain
// category. Intra-domain JOIN (groups ↔ group_quality).
func (s *Store) GetQualityGroupsForDomain(category string) ([]models.Group, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.platform, g.name, g.url, g.active, g.join_state,
		       COALESCE(g.last_scan,''), g.created_at
		FROM groups g
		JOIN group_quality gq ON gq.group_id = g.id
		WHERE g.active = 1
		  AND gq.blacklist = 0
		  AND gq.decision = 'use'
		  AND gq.category = ?
		ORDER BY gq.final_score DESC, gq.candidate_yield DESC`,
		category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}

// GetAllScoredGroups returns all groups with quality scores.
func (s *Store) GetAllScoredGroups() ([]models.GroupQuality, error) {
	rows, err := s.db.Query(`
		SELECT gq.group_id, g.name, g.url, gq.category, gq.relevance_score,
		       gq.professionalism_score, gq.content_quality_score, gq.spam_penalty,
		       gq.final_score, gq.decision, gq.reason, gq.whitelist, gq.blacklist,
		       COALESCE(gq.scored_at,''), COALESCE(gq.last_post_at,''),
		       gq.weekly_post_count, gq.candidate_yield, gq.spam_yield
		FROM group_quality gq JOIN groups g ON g.id = gq.group_id
		ORDER BY gq.final_score DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.GroupQuality
	for rows.Next() {
		var q models.GroupQuality
		var scoredAt, lastPostAt string
		if err := rows.Scan(&q.GroupID, &q.GroupName, &q.GroupURL, &q.Category,
			&q.RelevanceScore, &q.ProfessionalismScore, &q.ContentQualityScore, &q.SpamPenalty,
			&q.FinalScore, &q.Decision, &q.Reason, &q.Whitelist, &q.Blacklist,
			&scoredAt, &lastPostAt, &q.WeeklyPostCount, &q.CandidateYield, &q.SpamYield); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, nil
}

// MarkGroupWhitelist sets or clears the whitelist flag for a group.
func (s *Store) MarkGroupWhitelist(groupID int64, v bool) error {
	_, err := s.db.Exec(`INSERT INTO group_quality(group_id, whitelist) VALUES(?,?)
		ON CONFLICT(group_id) DO UPDATE SET whitelist=excluded.whitelist`, groupID, v)
	return err
}

// MarkGroupBlacklist sets or clears the blacklist flag and adjusts decision.
func (s *Store) MarkGroupBlacklist(groupID int64, v bool) error {
	decision := "monitor"
	if v {
		decision = "reject"
	}
	_, err := s.db.Exec(`INSERT INTO group_quality(group_id, blacklist, decision) VALUES(?,?,?)
		ON CONFLICT(group_id) DO UPDATE SET blacklist=excluded.blacklist, decision=excluded.decision`,
		groupID, v, decision)
	return err
}

// UpdateGroupYield increments candidate/spam yield counters and updates decision.
func (s *Store) UpdateGroupYield(groupID int64, qualityDelta, spamDelta int) error {
	_, err := s.db.Exec(`
		INSERT INTO group_quality(group_id, candidate_yield, spam_yield) VALUES(?,?,?)
		ON CONFLICT(group_id) DO UPDATE SET
			candidate_yield = candidate_yield + excluded.candidate_yield,
			spam_yield = spam_yield + excluded.spam_yield`,
		groupID, qualityDelta, spamDelta)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		UPDATE group_quality SET decision = CASE
			WHEN blacklist = 1 THEN 'reject'
			WHEN candidate_yield >= 3 AND (spam_yield * 1.0 / MAX(candidate_yield+spam_yield,1)) < 0.3 THEN 'use'
			WHEN spam_yield >= 5 AND (spam_yield * 1.0 / MAX(candidate_yield+spam_yield,1)) > 0.7 THEN 'reject'
			ELSE decision
		END WHERE group_id = ?`, groupID)
	return err
}

// UpdateGroupLastPost records that a post was sent to this group.
func (s *Store) UpdateGroupLastPost(groupID int64) error {
	_, err := s.db.Exec(`
		INSERT INTO group_quality(group_id, last_post_at, weekly_post_count) VALUES(?, CURRENT_TIMESTAMP, 1)
		ON CONFLICT(group_id) DO UPDATE SET
			last_post_at=CURRENT_TIMESTAMP,
			weekly_post_count=weekly_post_count+1`, groupID)
	return err
}

// GetUnscoredGroups returns active groups that have no quality record yet.
func (s *Store) GetUnscoredGroups() ([]models.Group, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.platform, g.name, g.url, g.active, g.join_state,
		       COALESCE(g.last_scan,''), g.created_at
		FROM groups g
		LEFT JOIN group_quality gq ON gq.group_id = g.id
		WHERE g.active = 1 AND gq.group_id IS NULL
		ORDER BY g.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupRows(rows)
}
