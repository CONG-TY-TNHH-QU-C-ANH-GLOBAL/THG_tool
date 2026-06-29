package main

import (
	"context"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// storeLeadCoverage adapts *store.Store to leadoutreach.LeadCoverageReader. Pure
// pass-through: it relays the existing Leads().GetLeadCoverageState read verbatim —
// no behavior change, the existing store path stays authoritative.
type storeLeadCoverage struct{ db *store.Store }

func (r storeLeadCoverage) GetLeadCoverageState(ctx context.Context, orgID, leadID int64, website string) (*models.LeadCoverageState, error) {
	return r.db.Leads().GetLeadCoverageState(ctx, orgID, leadID, website)
}

// storeLeadLifecycle adapts *store.Store to leadoutreach.LeadLifecycleReader. Pure
// pass-through.
type storeLeadLifecycle struct{ db *store.Store }

func (r storeLeadLifecycle) LeadLifecycleSummary(ctx context.Context, orgID int64) (models.LifecycleSummary, error) {
	return r.db.Leads().LeadLifecycleSummary(ctx, orgID)
}
