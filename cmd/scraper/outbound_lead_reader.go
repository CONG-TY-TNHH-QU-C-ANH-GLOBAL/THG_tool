package main

import (
	"context"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// leadCoverageReader reads the multi-actor coverage state for a lead. It is the
// SECOND ARCHCM2c seam: it removes the per-lead pipeline's direct *store.Store
// coverage read (coverageGate) so the movable execution path no longer names a
// concrete store for this lookup. One read-only method, neutral return
// (*models.LeadCoverageState) — not a re-export of the store surface.
//
// On the eventual move this interface travels WITH the cluster (consumer-owned);
// storeLeadCoverage below stays in cmd as the adapter.
type leadCoverageReader interface {
	GetLeadCoverageState(ctx context.Context, orgID, leadID int64, website string) (*models.LeadCoverageState, error)
}

// storeLeadCoverage adapts *store.Store to leadCoverageReader. Pure pass-through:
// it relays the existing Leads().GetLeadCoverageState read verbatim — no behavior
// change, the existing store path stays authoritative.
type storeLeadCoverage struct{ db *store.Store }

func (r storeLeadCoverage) GetLeadCoverageState(ctx context.Context, orgID, leadID int64, website string) (*models.LeadCoverageState, error) {
	return r.db.Leads().GetLeadCoverageState(ctx, orgID, leadID, website)
}
