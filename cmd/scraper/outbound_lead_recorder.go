package main

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// queueOutcome is the store-free result of a queued outbound write, carrying
// only the fields the lead-outreach path actually reads. It exists so
// outboundRecorder does not leak internal/store/outbound.QueueResult (a store-tree
// type) into the consumer — a prerequisite for relocating the lead pipeline onto
// the FB usecase side (ARCHCM2c), which may not import the store tree. Every
// field is neutral (internal/models + primitives).
type queueOutcome struct {
	ExecutionState models.ExecutionState
	Allowed        bool
	Reason         string
	RiskScore      float64
	RiskCeiling    float64
}

// outboundRecorder is the narrow port the lead-outcome path uses to persist a
// queued message and record its per-stage Knowledge OS outcome. It is the FIRST
// ARCHCM2c decoupling seam: it replaces the cluster's two direct *store.Store
// calls (QueueOutboundForOrg + Knowledge().RecordOutcome) so the outcome path no
// longer names a concrete store. Two methods only — deliberately NOT a re-export
// of the store surface (no "god interface"). The concrete store still executes
// the queue write, so queue/dedup/policy semantics are unchanged.
//
// On the eventual move this interface + queueOutcome travel WITH the cluster
// (consumer-owned); storeOutboundRecorder below stays in cmd as the adapter.
type outboundRecorder interface {
	QueueOutbound(msg *models.OutboundMessage, cooldown time.Duration) (queueOutcome, error)
	RecordOutcome(ctx context.Context, orgID int64, retrievalID, status string)
}

// storeOutboundRecorder adapts *store.Store to outboundRecorder. Pure
// pass-through: it relays the existing QueueOutboundForOrg / Knowledge().RecordOutcome
// calls verbatim and maps the store result into queueOutcome 1:1 — no behavior
// change, the existing store path stays authoritative.
type storeOutboundRecorder struct{ db *store.Store }

func (r storeOutboundRecorder) QueueOutbound(msg *models.OutboundMessage, cooldown time.Duration) (queueOutcome, error) {
	// Non-deprecated L2 path (QueueOutboundForOrg is a deprecated wrapper over this).
	res, err := r.db.Outbound().Queue(msg, cooldown)
	if err != nil {
		return queueOutcome{}, err
	}
	return queueOutcome{
		ExecutionState: res.ExecutionState,
		Allowed:        res.Decision.Allowed,
		Reason:         res.Decision.Reason,
		RiskScore:      res.Decision.RiskScore,
		RiskCeiling:    res.Decision.RiskCeiling,
	}, nil
}

func (r storeOutboundRecorder) RecordOutcome(ctx context.Context, orgID int64, retrievalID, status string) {
	r.db.Knowledge().RecordOutcome(ctx, orgID, retrievalID, status)
}

// storePromptLog adapts *store.Store to commenting.SystemPromptLogInserter (ARCHCM2c seam 4),
// so the commenting usecase records its decision log without taking a *store.Store.
// Pure pass-through over Prompts().InsertSystemPromptLog.
type storePromptLog struct{ db *store.Store }

func (s storePromptLog) InsertSystemPromptLog(orgID, accountID int64, message, action, args string, success bool) error {
	return s.db.Prompts().InsertSystemPromptLog(orgID, accountID, message, action, args, success)
}
