package facebook

import (
	"context"

	"github.com/thg/scraper/internal/models"
)

// LeadSource is the narrow, consumer-owned port the FB outreach planner needs to
// read leads. services/facebook owns this interface and depends ONLY on it (plus
// models) — it does not import internal/store. The composition root (cmd/scraper)
// supplies a tiny adapter backed by *store.Store. Methods are tenant-scoped by
// orgID and named by capability, not storage mechanics.
type LeadSource interface {
	// LeadByID returns the one org-scoped lead, or nil if not found in this org.
	LeadByID(ctx context.Context, orgID, leadID int64) (*models.Lead, error)
	// WorkQueueLeads returns the lifecycle-filtered act-now candidate pool
	// (ordered by score → freshness → next_action_at), capped at poolSize.
	WorkQueueLeads(ctx context.Context, orgID int64, scoreFilter string, poolSize int) ([]models.Lead, error)
}

// LeadSelectionInput is the parsed, transport-free request for lead selection.
// The composition root extracts these from the raw action args so this service
// never sees the untyped map[string]any.
type LeadSelectionInput struct {
	LeadID      int64
	PostURL     string
	TargetURL   string
	TargetName  string
	AuthorURL   string
	Context     string
	ScoreFilter string
	Limit       int
	MaxItems    int
}

// LeadsForAction resolves the lead(s) an outbound action will run against, in
// precedence order (behavior moved verbatim from cmd/scraper):
//
//  1. §7 direct-link: a single existing org-scoped lead by LeadID (real content +
//     coverage history, not a synthetic shell). Not found → no leads.
//  2. FB synthetic prompt_target lead shaped from the post/target URL fields.
//  3. WorkQueue fallback: the lifecycle-filtered act-now pool (inbox defaults to
//     the "hot" score band), sized for eligible-fill.
func LeadsForAction(ctx context.Context, src LeadSource, orgID int64, msgType string, in LeadSelectionInput) ([]models.Lead, error) {
	if in.LeadID > 0 {
		lead, err := src.LeadByID(ctx, orgID, in.LeadID)
		if err != nil {
			return nil, err
		}
		if lead == nil {
			return nil, nil
		}
		return []models.Lead{*lead}, nil
	}
	// Facebook-specific synthetic-lead shaping (prompt_target conventions). Empty
	// result = no prompt target → fall through to the work-queue selection below.
	if lead, ok := SyntheticLeadFromActionArgs(
		orgID, msgType,
		in.PostURL, in.TargetURL,
		in.TargetName, in.AuthorURL,
		in.Context,
	); ok {
		return []models.Lead{lead}, nil
	}
	score := in.ScoreFilter
	if score == "" && msgType == "inbox" {
		score = "hot"
	}
	return src.WorkQueueLeads(ctx, orgID, score, scanPoolFor(RequestedOutreachCount(in.Limit, in.MaxItems)))
}

// RequestedOutreachCount is how many ELIGIBLE comments/messages the caller asked
// to queue ("comment thử 5 lead" → 5). Prefers limit, then the agent's max_items
// fallback; defaults to 25.
func RequestedOutreachCount(limit, maxItems int) int {
	n := limit
	if n <= 0 {
		n = maxItems
	}
	if n <= 0 {
		n = 25
	}
	return n
}

// scanPoolFor sizes the candidate pool so the planner can keep scanning past
// skipped leads until it has queued `requested` eligible comments —
// max(50, requested*10).
func scanPoolFor(requested int) int {
	if n := requested * 10; n > 50 {
		return n
	}
	return 50
}
