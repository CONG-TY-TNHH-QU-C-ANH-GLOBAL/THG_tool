package leads

import (
	"context"

	"github.com/thg/scraper/internal/models"
)

// Lead Lifecycle read endpoints (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-4). Batch
// projection for the dashboard list view + the archived-leads read path. Split from
// lead_lifecycle.go to keep each file under the size budget.

// GetLeadLifecyclesBatch projects lifecycle state for many leads (dashboard list-view
// enrichment). Keyed by lead id; missing/inaccessible ids are simply omitted so the caller
// defaults them. Bounded by the caller (handler caps ids per call).
func (s *Store) GetLeadLifecyclesBatch(ctx context.Context, orgID int64, leadIDs []int64, policy models.LeadLifecyclePolicy) (map[int64]models.LeadLifecycleState, error) {
	out := make(map[int64]models.LeadLifecycleState, len(leadIDs))
	for _, id := range leadIDs {
		st, err := s.GetLeadLifecycle(ctx, orgID, id, policy)
		if err != nil {
			continue
		}
		out[id] = *st
	}
	return out, nil
}

// ListArchivedLeads returns the org's archived leads, most-recently-archived first — the
// "Đã lưu trữ" tab. The default list (GetLeadsFiltered) excludes these; this is the only
// read path that surfaces them. Lean projection (no commented/EXISTS subquery): the
// archived view is informational. // tenant-ok: cross-domain projection (leads -> crawl)
func (s *Store) ListArchivedLeads(ctx context.Context, orgID int64, limit, offset int) ([]models.Lead, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT l.id, COALESCE(l.org_id,0), l.source_type, l.source_id,
		        COALESCE(NULLIF(l.source_url,''), p.url, ''), COALESCE(l.secondary_url,''),
		        COALESCE(l.post_fbid,''), COALESCE(l.comment_fbid,''), COALESCE(l.group_fbid,''),
		        l.platform, COALESCE(l.author,''), COALESCE(l.author_url,''), l.content, l.score,
		        COALESCE(l.service_match,''), COALESCE(l.author_role,''), COALESCE(l.pain_point,''),
		        COALESCE(l.ai_reasoning,''), COALESCE(NULLIF(l.niche,''),'logistics'),
		        COALESCE(NULLIF(l.thread_role,''),'intent_originator'), l.created_at
		   FROM leads l LEFT JOIN posts p ON l.source_id = p.id
		  WHERE COALESCE(l.org_id,0) = ? AND l.archived_at IS NOT NULL
		  ORDER BY l.archived_at DESC LIMIT ? OFFSET ?`,
		orgID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var leads []models.Lead
	for rows.Next() {
		var l models.Lead
		if err := rows.Scan(&l.ID, &l.OrgID, &l.SourceType, &l.SourceID, &l.SourceURL, &l.SecondaryURL,
			&l.PostFBID, &l.CommentFBID, &l.GroupFBID, &l.Platform, &l.Author, &l.AuthorURL, &l.Content,
			&l.Score, &l.ServiceMatch, &l.AuthorRole, &l.PainPoint, &l.AIReasoning, &l.Niche,
			&l.ThreadRole, &l.CreatedAt); err != nil {
			return nil, err
		}
		repairLeadSourceURL(&l)
		leads = append(leads, l)
	}
	return leads, rows.Err()
}
