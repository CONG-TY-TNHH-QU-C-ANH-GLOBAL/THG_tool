package leads

import (
	"context"

	"github.com/thg/scraper/internal/models"
)

// lifecycleSummaryScan bounds how many leads the summary projects per org — the copilot
// "nothing eligible" path is rare, so a generous-but-bounded scan is fine.
const lifecycleSummaryScan = 1000

// LeadLifecycleSummary tallies an org's leads by freshness state for the copilot
// suggestion (PR-5). Live states come from the work-queue projection; archived is an
// exact, cheap count (archived rows are excluded from the projection upstream).
func (s *Store) LeadLifecycleSummary(ctx context.Context, orgID int64) (models.LifecycleSummary, error) {
	var sum models.LifecycleSummary
	items, err := s.GetWorkQueue(ctx, orgID, models.WorkQueueOptions{
		States: []models.LeadFreshnessState{
			models.LeadActive, models.LeadWaitingReply,
			models.LeadWaitingVerification, models.LeadFollowupDue,
		},
		IncludeStale: true,
		Limit:        lifecycleSummaryScan,
	})
	if err != nil {
		return sum, err
	}
	for _, it := range items {
		switch it.Lifecycle.FreshnessState {
		case models.LeadActive:
			sum.Active++
		case models.LeadWaitingReply:
			sum.WaitingReply++
		case models.LeadWaitingVerification:
			sum.WaitingVerification++
		case models.LeadFollowupDue:
			sum.FollowupDue++
		case models.LeadStale:
			sum.Stale++
		}
	}
	archived, err := s.countArchivedLeads(ctx, orgID)
	if err != nil {
		return sum, err
	}
	sum.Archived = archived
	return sum, nil
}

func (s *Store) countArchivedLeads(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM leads WHERE COALESCE(org_id,0) = ? AND archived_at IS NOT NULL`,
		orgID).Scan(&n)
	return n, err
}
