package main

import (
	"context"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
)

// Composition-root adapters wiring *store.Store + raw action args into the
// consumer-owned ports of internal/services/facebook (PR29G). These live here
// (the wiring boundary) so services/facebook stays free of internal/store and the
// untyped action-args map. Thin pass-throughs / field shaping only — no logic,
// no behavior change.

// fbLeadSource adapts *store.Store to facebook.LeadSource.
type fbLeadSource struct{ db *store.Store }

func (s fbLeadSource) LeadByID(ctx context.Context, orgID, leadID int64) (*models.Lead, error) {
	return s.db.Leads().GetLeadByID(ctx, orgID, leadID)
}

func (s fbLeadSource) WorkQueueLeads(ctx context.Context, orgID int64, scoreFilter string, poolSize int) ([]models.Lead, error) {
	return s.db.Leads().WorkQueueLeads(ctx, orgID, scoreFilter, poolSize)
}

// Compile-time check: the adapter satisfies the consumer-owned port.
var _ facebook.LeadSource = fbLeadSource{}

// leadSelectionInputFromArgs parses the transport-level action args into the
// typed facebook.LeadSelectionInput (the args map stays in cmd/scraper).
func leadSelectionInputFromArgs(args map[string]any) facebook.LeadSelectionInput {
	return facebook.LeadSelectionInput{
		LeadID:      argInt64(args, "lead_id"),
		PostURL:     argString(args, "post_url"),
		TargetURL:   argString(args, "target_url"),
		TargetName:  argString(args, "target_name"),
		AuthorURL:   argString(args, "author_url"),
		Context:     argString(args, "context"),
		ScoreFilter: argString(args, "score_filter"),
		Limit:       int(argInt64(args, "limit")),
		MaxItems:    int(argInt64(args, "max_items")),
	}
}

// postContentInputFromArgs parses the transport-level action args into the typed
// facebook.FacebookPostContentInput.
func postContentInputFromArgs(args map[string]any) facebook.FacebookPostContentInput {
	return facebook.FacebookPostContentInput{
		Content:      argString(args, "content"),
		Description:  argString(args, "description"),
		Title:        argString(args, "title"),
		Requirements: argString(args, "requirements"),
		Benefits:     argString(args, "benefits"),
		Salary:       argString(args, "salary"),
		Email:        argString(args, "email"),
	}
}
