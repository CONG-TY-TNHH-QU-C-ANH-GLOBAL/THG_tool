package main

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook/leadoutreach"
	"github.com/thg/scraper/internal/store"
)

// storeOutboundRecorder adapts *store.Store to leadoutreach.OutboundRecorder. Pure
// pass-through: it relays the existing QueueOutboundForOrg / Knowledge().RecordOutcome
// calls verbatim and maps the store result into leadoutreach.QueueOutcome 1:1 — no
// behavior change, the existing store path stays authoritative.
type storeOutboundRecorder struct{ db *store.Store }

func (r storeOutboundRecorder) QueueOutbound(msg *models.OutboundMessage, cooldown time.Duration) (leadoutreach.QueueOutcome, error) {
	// Non-deprecated L2 path (QueueOutboundForOrg is a deprecated wrapper over this).
	res, err := r.db.Outbound().Queue(msg, cooldown)
	if err != nil {
		return leadoutreach.QueueOutcome{}, err
	}
	return leadoutreach.QueueOutcome{
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
