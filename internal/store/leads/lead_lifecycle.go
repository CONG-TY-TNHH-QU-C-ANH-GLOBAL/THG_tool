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
		LastSoftTouchAt:     s.latestSoftTouchAt(ctx, orgID, row.matchURLs()),
		LastCustomerReplyAt: s.lastCustomerReplyAt(orgID, row.authorURL),
		ThreadStatus:        eng.ThreadStatus,
		ArchivedAt:          row.archivedAt,
		ArchiveReason:       row.archiveReason,
	}
	st := models.DeriveLeadLifecycle(in, policy, time.Now())
	return &st, nil
}

// latestSoftTouchAt returns when a SUBMITTED-but-unverified comment last landed on any of
// the lead's target URLs (action_ledger outcome='submitted_unverified'). This is the soft
// touch that holds the lead in verification cooldown — distinct from the verified-touch
// ('succeeded') path the engagement projection reads. // tenant-ok: leads -> coordination
func (s *Store) latestSoftTouchAt(ctx context.Context, orgID int64, urls []string) time.Time {
	if len(urls) == 0 {
		return time.Time{}
	}
	placeholders := strings.Repeat("?,", len(urls))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, 0, len(urls)+2)
	args = append(args, orgID, models.LedgerOutcomeSubmittedUnverified)
	for _, u := range urls {
		args = append(args, u)
	}
	var at sql.NullTime
	// Read the column directly (ORDER BY ... LIMIT 1), NOT MAX(): an aggregate loses the
	// datetime type affinity under modernc.org/sqlite and scans as an unparseable string.
	err := s.db.QueryRowContext(ctx,
		`SELECT performed_at FROM action_ledger
		  WHERE org_id = ? AND outcome = ? AND action_type = 'comment'
		    AND target_url IN (`+placeholders+`)
		  ORDER BY performed_at DESC LIMIT 1`,
		args...,
	).Scan(&at)
	if err != nil || !at.Valid {
		return time.Time{}
	}
	return at.Time
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
	sourceURL     string
	authorURL     string
	secondaryURL  string
	createdAt     time.Time
	archivedAt    time.Time
	archiveReason string
}

// matchURLs returns the lead's engagement target URLs (for ledger lookups).
func (r lifecycleRow) matchURLs() []string {
	out := make([]string, 0, 3)
	for _, u := range []string{r.sourceURL, r.authorURL, r.secondaryURL} {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// getLeadLifecycleRow reads the crawl + archive signals for one org-scoped lead in a
// single round-trip.
func (s *Store) getLeadLifecycleRow(ctx context.Context, orgID, leadID int64) (lifecycleRow, error) {
	var (
		r          lifecycleRow
		archivedAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(source_url,''), COALESCE(author_url,''), COALESCE(secondary_url,''),
		        created_at, archived_at, COALESCE(archive_reason,'')
		   FROM leads
		  WHERE id = ? AND COALESCE(org_id,0) = ?`,
		leadID, orgID,
	).Scan(&r.sourceURL, &r.authorURL, &r.secondaryURL, &r.createdAt, &archivedAt, &r.archiveReason)
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
