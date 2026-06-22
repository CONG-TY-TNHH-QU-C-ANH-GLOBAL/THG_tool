package agent

import (
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/system"
)

// claimCandidate applies the per-candidate eligibility gates and, when eligible,
// claims one planned outbound for this worker. It is the former inline body of
// agentGetOutbox's candidate loop, extracted verbatim — same checks, same order,
// same side effects, with the message passed and returned BY VALUE (the loop
// never takes the address of the range variable).
//
// Returns:
//   - (claimed, true, nil)  — claimed; ExecutionState/ExecutionID stamped and the
//     execution_started activity event emitted.
//   - (msg, false, nil)     — skip this candidate (not eligible / claim lost).
//   - (msg, false, err)     — ownership-store failure the caller must surface as
//     500 (mirrors the former inline `return c.Status(500)` on this path).
func (h *Handler) claimCandidate(orgID, agentID, assignedAccountID int64, workerID string, msg models.OutboundMessage) (models.OutboundMessage, bool, error) {
	if msg.AccountID <= 0 {
		return msg, false, nil
	}
	if assignedAccountID > 0 && msg.AccountID != assignedAccountID {
		return msg, false, nil
	}
	ownsStream, err := h.db.Connectors().ConnectorOwnsAccountStream(orgID, agentID, msg.AccountID)
	if err != nil {
		return msg, false, err
	}
	if !ownsStream {
		return msg, false, nil
	}
	claim, err := h.db.ClaimPlannedOutboundForOrg(orgID, msg.ID, workerID, 0)
	if err != nil || claim == nil {
		return msg, false, nil
	}
	msg.ExecutionState = models.ExecExecuting
	msg.ExecutionID = claim.ExecutionID
	// Activity feed: execution_started — the autonomous-first vocabulary makes
	// "extension claimed and is about to mutate the live DOM" a distinct event
	// from "intent was queued" (execution_planned) and from terminal events
	// (execution_verified / execution_failed).
	system.NotifyExecutionStarted(h.db, orgID, msg.AccountID, msg.ID, claim.ExecutionID, msg.Type)
	return msg, true, nil
}
