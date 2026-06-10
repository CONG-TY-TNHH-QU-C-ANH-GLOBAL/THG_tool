package leads

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// GetLeadCoverageState projects the multi-actor coverage picture for a lead from the
// VERIFIED engagement ledger + the conversation thread (spec:
// specs/MULTI_ACTOR_COVERAGE_POLICY.md). The planner uses it to decide whether ANOTHER
// actor may cover this lead. This projection owns the ledger + reply truth; the
// content-derived fields (website/CTA/angles) are filled by the generation layer.
func (s *Store) GetLeadCoverageState(ctx context.Context, orgID, leadID int64) (*models.LeadCoverageState, error) {
	eng, err := s.GetLeadEngagement(ctx, orgID, leadID)
	if err != nil {
		return nil, err
	}
	st := models.ProjectLeadCoverage(eng.Entries, s.leadHasReplied(ctx, orgID, leadID))
	return &st, nil
}

// leadHasReplied reports whether the lead has sent us an inbound reply (engagement
// back), keyed on the lead's profile URL — the StopIfLeadReplies signal. // tenant-ok
func (s *Store) leadHasReplied(ctx context.Context, orgID, leadID int64) bool {
	lead, err := s.getLeadForOrg(ctx, orgID, leadID)
	if err != nil || lead == nil {
		return false
	}
	url := strings.TrimSpace(lead.AuthorURL)
	if url == "" {
		return false
	}
	thread, err := s.Threads().GetThreadByProfileForOrg(orgID, url)
	if err != nil || thread == nil {
		return false
	}
	return !thread.LastInboundAt.IsZero()
}
