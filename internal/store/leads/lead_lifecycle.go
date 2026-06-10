package leads

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Lead Lifecycle projection + archive writers (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md).
// freshness_state / next_action are DERIVED from the verified engagement ledger + the
// conversation thread (never stored); only archived_at + archive_reason are persisted.
// This file is the store-side wiring: it gathers the explicit signals and hands them to
// the pure models.DeriveLeadLifecycle.

// GetLeadLifecycle projects the work-management lifecycle state for one lead. It reuses
// the verified-touch engagement projection (LastEngagedAt + thread status) and reads the
// crawl + archive timestamps from the canonical leads row.
func (s *Store) GetLeadLifecycle(ctx context.Context, orgID, leadID int64, policy models.LeadLifecyclePolicy) (*models.LeadLifecycleState, error) {
	row, err := s.getLeadLifecycleRow(ctx, orgID, leadID)
	if err != nil {
		return nil, err
	}
	eng, err := s.GetLeadEngagement(ctx, orgID, leadID)
	if err != nil {
		return nil, err
	}
	in := models.LeadLifecycleInputs{
		LeadID:              leadID,
		LastCrawledAt:       row.createdAt,
		LastEngagedAt:       eng.LastEngagedAt,
		LastCustomerReplyAt: s.lastCustomerReplyAt(orgID, row.authorURL),
		ThreadStatus:        eng.ThreadStatus,
		ArchivedAt:          row.archivedAt,
		ArchiveReason:       row.archiveReason,
	}
	st := models.DeriveLeadLifecycle(in, policy, time.Now())
	return &st, nil
}

// GetLeadLifecyclesBatch projects lifecycle state for many leads (dashboard list-view
// enrichment, PR-4). Keyed by lead id; missing/inaccessible ids are simply omitted so the
// caller defaults them. Bounded by the caller (handler caps ids per call).
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

// ArchiveLead marks a lead archived with a typed reason. Idempotent: the
// `archived_at IS NULL` guard means a second archive never overwrites the original
// timestamp/reason. No hard delete — the row (and its engagement ledger) stay for
// dedup + multi-actor coverage history; it is only hidden from the default list.
func (s *Store) ArchiveLead(ctx context.Context, orgID, leadID int64, reason string) error {
	if orgID <= 0 || leadID <= 0 {
		return fmt.Errorf("archive requires org_id + lead_id")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE leads
		    SET archived_at = CURRENT_TIMESTAMP, archive_reason = ?
		  WHERE id = ? AND COALESCE(org_id,0) = ? AND archived_at IS NULL`,
		strings.TrimSpace(reason), leadID, orgID,
	)
	return err
}

// UnarchiveLead clears the archive decision, returning the lead to the live list.
func (s *Store) UnarchiveLead(ctx context.Context, orgID, leadID int64) error {
	if orgID <= 0 || leadID <= 0 {
		return fmt.Errorf("unarchive requires org_id + lead_id")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE leads SET archived_at = NULL, archive_reason = ''
		  WHERE id = ? AND COALESCE(org_id,0) = ?`,
		leadID, orgID,
	)
	return err
}

// lifecycleRow is the minimal lead projection input read directly from the leads table.
type lifecycleRow struct {
	authorURL     string
	createdAt     time.Time
	archivedAt    time.Time
	archiveReason string
}

// getLeadLifecycleRow reads the crawl + archive signals for one org-scoped lead in a
// single round-trip.
func (s *Store) getLeadLifecycleRow(ctx context.Context, orgID, leadID int64) (lifecycleRow, error) {
	var (
		r          lifecycleRow
		archivedAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(author_url,''), created_at, archived_at, COALESCE(archive_reason,'')
		   FROM leads
		  WHERE id = ? AND COALESCE(org_id,0) = ?`,
		leadID, orgID,
	).Scan(&r.authorURL, &r.createdAt, &archivedAt, &r.archiveReason)
	if err != nil {
		return lifecycleRow{}, err
	}
	if archivedAt.Valid {
		r.archivedAt = archivedAt.Time
	}
	return r, nil
}

// lastCustomerReplyAt returns when the lead last sent us an inbound reply, keyed on the
// lead's profile URL — the same thread signal leadReplied uses, but timestamped so the
// lifecycle can tell waiting_reply from a live conversation. // tenant-ok: leads -> threads
func (s *Store) lastCustomerReplyAt(orgID int64, authorURL string) time.Time {
	url := strings.TrimSpace(authorURL)
	if url == "" {
		return time.Time{}
	}
	thread, err := s.Threads().GetThreadByProfileForOrg(orgID, url)
	if err != nil || thread == nil {
		return time.Time{}
	}
	return thread.LastInboundAt
}
