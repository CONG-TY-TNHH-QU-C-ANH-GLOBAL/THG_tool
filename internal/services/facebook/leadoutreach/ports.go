package leadoutreach

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
)

// QueueOutcome is the store-free result of a queued outbound write, carrying
// only the fields the lead-outreach path actually reads. It exists so
// OutboundRecorder does not leak internal/store/outbound.QueueResult (a store-tree
// type) into the consumer — a prerequisite for relocating the lead pipeline onto
// the FB usecase side (ARCHCM2c), which may not import the store tree. Every
// field is neutral (internal/models + primitives).
type QueueOutcome struct {
	ExecutionState models.ExecutionState
	Allowed        bool
	Reason         string
	RiskScore      float64
	RiskCeiling    float64
}

// OutboundRecorder is the narrow port the lead-outcome path uses to persist a
// queued message and record its per-stage Knowledge OS outcome. It is the FIRST
// ARCHCM2c decoupling seam: it replaces the cluster's two direct *store.Store
// calls (QueueOutboundForOrg + Knowledge().RecordOutcome) so the outcome path no
// longer names a concrete store. Two methods only — deliberately NOT a re-export
// of the store surface (no "god interface"). The concrete store still executes
// the queue write, so queue/dedup/policy semantics are unchanged.
//
// The adapter (storeOutboundRecorder) stays in cmd as the composition root.
type OutboundRecorder interface {
	QueueOutbound(msg *models.OutboundMessage, cooldown time.Duration) (QueueOutcome, error)
	RecordOutcome(ctx context.Context, orgID int64, retrievalID, status string)
}

// LeadCoverageReader reads the multi-actor coverage state for a lead. It is the
// SECOND ARCHCM2c seam: it removes the per-lead pipeline's direct *store.Store
// coverage read (coverageGate) so the movable execution path no longer names a
// concrete store for this lookup. One read-only method, neutral return
// (*models.LeadCoverageState) — not a re-export of the store surface.
//
// The adapter (storeLeadCoverage) stays in cmd as the composition root.
type LeadCoverageReader interface {
	GetLeadCoverageState(ctx context.Context, orgID, leadID int64, website string) (*models.LeadCoverageState, error)
}

// LeadLifecycleReader reads the per-org lead lifecycle summary. It is the THIRD
// ARCHCM2c seam: it removes the last direct *store.Store read in the outcome path
// (noEligibleCommentMessage), so the outcome path is fully store-free. One
// read-only method, neutral return (models.LifecycleSummary).
//
// The adapter (storeLeadLifecycle) stays in cmd as the composition root.
type LeadLifecycleReader interface {
	LeadLifecycleSummary(ctx context.Context, orgID int64) (models.LifecycleSummary, error)
}
