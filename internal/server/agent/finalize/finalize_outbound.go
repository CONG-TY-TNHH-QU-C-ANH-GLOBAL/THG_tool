package finalize

import (
	"context"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
)

// Outbound finalization state machine: the orchestrator, its per-callback state
// struct, and the terminal-decision steps (load → enforce identity → execution_id
// CAS). The FIRST-WIN side effects live in finalize_side_effects.go; the terminal
// response, notifications, and evidence/proof adapters in finalize_helpers.go.

// FinalizeOutbound is the single write-point for terminal outbound
// callbacks. It encodes three invariants:
//
//  1. EXECUTION IDENTITY (post-hoc defense for wrong-post bug class):
//     EnforceTargetIdentity downgrades success-class outcomes to
//     ContextDrift when the extension's reported page_url_after does
//     not address the same Facebook entity as the queued target_url.
//
//  2. TERMINAL CAS (lease + execution_id idempotency):
//     outbound.Store.Finalize requires (status='sending', execution_id
//     matches OR row's execution_id is empty for legacy rows). A
//     replayed callback hitting an already-terminal row returns
//     idempotent-OK; a callback whose execution_id no longer matches
//     the row (because outbound.Store.ResetStaleExecuting lease-evicted
//     it and a new claim issued a fresh token) returns 409 stale.
//
//  3. SIDE EFFECTS ARE COMMITTED ONLY ON FIRST-WIN:
//     execution_attempts row, action_ledger update, and risk-signal
//     application happen INSIDE the finalized==true branch only.
//     Replays do NOT replay them — that was the whole point of
//     introducing execution_id. The previous architecture wrote side
//     effects unconditionally on every /sent or /failed hit and so
//     would have multiplied evidence rows on SW-restart-triggered
//     duplicate callbacks.
//
// Errors from the side-effect writes are logged but never propagated
// — they are verification telemetry, not the load-bearing path.
func (h *Handler) FinalizeOutbound(
	c *fiber.Ctx,
	orgID, id int64,
	report runtime.ExtensionExecutionReport,
	outcome models.ExecutionOutcome,
	proof runtime.VerifierProof,
) (*FinalizeResolution, error) {
	ctx := c.UserContext()
	f := &outboundFinalizer{
		h:       h,
		orgID:   orgID,
		id:      id,
		report:  report,
		outcome: outcome,
		proof:   proof,
	}
	if res := f.loadOutbound(); res != nil {
		return res, nil
	}
	f.enforceTargetIdentity(ctx)
	if res, err := f.attemptFinalization(ctx); res != nil || err != nil {
		return res, err
	}
	// FIRST-WIN PATH — commit side effects exactly once (the !finalized
	// replay/stale callbacks returned above, so none of the below can run twice).
	f.applyFirstWinSideEffects(ctx)
	f.refundQuotaForFailure(ctx)
	f.upsertInboxThreadForSuccess(ctx)
	f.notifyOutboundFinalized()
	return f.buildResponse(), nil
}

// outboundFinalizer carries the per-callback state for one terminal /sent or
// /failed finalization. It is created and used once per FinalizeOutbound call and
// never reused/stored; mutating methods use a pointer receiver (notably proof is
// mutated by persistFailureEvidence before it rides into evidence/notification).
// The request context is NOT stored on the struct — it is passed explicitly to
// the methods that perform store/runtime operations, as the first parameter.
type outboundFinalizer struct {
	h               *Handler
	orgID           int64
	id              int64
	report          runtime.ExtensionExecutionReport
	outcome         models.ExecutionOutcome
	proof           runtime.VerifierProof
	msg             *models.OutboundMessage
	terminalState   models.ExecutionState
	terminalOutcome models.VerificationOutcome
	attemptID       int64
	failureReason   string
}

// loadOutbound fetches the outbound row. A missing row yields the 404 resolution
// the caller returns verbatim; otherwise it caches the row and returns nil.
func (f *outboundFinalizer) loadOutbound() *FinalizeResolution {
	msg, msgErr := f.h.db.Outbound().Get(f.orgID, f.id)
	if msgErr != nil {
		return &FinalizeResolution{
			HTTPStatus: 404,
			Body:       fiber.Map{"error": "outbound message not found"},
		}
	}
	f.msg = msg
	return nil
}

// enforceTargetIdentity applies the defense-in-depth identity downgrade
// (success-class → ContextDrift on target_url/page_url_after entity mismatch) and
// emits the non-success diagnostic log line (precise landing cause via NavDiagnostic).
func (f *outboundFinalizer) enforceTargetIdentity(ctx context.Context) {
	f.outcome, f.proof = runtime.EnforceTargetIdentity(f.outcome, f.proof, f.msg.TargetURL, f.msg.Type)
	if !models.IsSuccessOutcome(f.outcome) {
		redirectClass, navStage, landedURL := "", "", ""
		if f.proof.NavDiagnostic != nil {
			redirectClass = f.proof.NavDiagnostic.RedirectClass
			navStage = f.proof.NavDiagnostic.Stage
			landedURL = f.proof.NavDiagnostic.LandedURL
		}
		slog.WarnContext(ctx, "exec-verify: non-success outcome",
			"org_id", f.orgID, "outbound_id", f.id,
			"account_id", f.msg.AccountID,
			"target_url", f.msg.TargetURL,
			"outcome", string(f.outcome),
			"failure_reason", f.report.FailureReason,
			"redirect_class", redirectClass,
			"nav_stage", navStage,
			"landed_url", landedURL,
			"page_url_after", f.proof.PageURLAfter,
			"notes", f.proof.Notes,
			"dom_snippet", f.proof.DOMSnippet,
		)
	}
}

// attemptFinalization computes the terminal (state, verification_outcome) pair and
// runs the execution_id-gated CAS. It returns a non-nil resolution for the two
// non-first-win terminals (stale 409 / idempotent replay 200), a non-nil error for
// a CAS failure, or (nil, nil) when THIS callback won the terminal transition.
func (f *outboundFinalizer) attemptFinalization(ctx context.Context) (*FinalizeResolution, error) {
	f.terminalState, f.terminalOutcome = models.TerminalFromOutcome(f.outcome)

	finalized, currentState, currentOutcome, currentExecID, err := f.h.db.Outbound().Finalize(ctx, f.orgID, f.id, f.report.ExecutionID, f.terminalState, f.terminalOutcome)
	if err != nil {
		return nil, err
	}
	if finalized {
		return nil, nil
	}
	// Disambiguate: replay (same token, already terminal) vs stale (token
	// mismatch, row was re-claimed).
	if f.report.ExecutionID != "" && currentExecID != "" && f.report.ExecutionID != currentExecID {
		// Stale: the row was lease-evicted and re-claimed; this callback belongs
		// to an execution that no longer owns the row. Refuse loudly — 409.
		slog.WarnContext(ctx, "exec-verify: stale execution_id",
			"org_id", f.orgID, "outbound_id", f.id,
			"submitted_execution_id", f.report.ExecutionID,
			"current_execution_id", currentExecID,
			"current_state", currentState,
			"current_outcome", currentOutcome,
		)
		return &FinalizeResolution{
			HTTPStatus: 409,
			Body: fiber.Map{
				"error":                "stale execution_id",
				"current_state":        string(currentState),
				"current_outcome":      string(currentOutcome),
				"current_execution_id": currentExecID,
			},
		}, nil
	}
	// Idempotent replay: same execution_id, row already terminal. Return
	// success-shaped response WITHOUT replaying side effects.
	slog.InfoContext(ctx, "exec-verify: idempotent replay",
		"org_id", f.orgID, "outbound_id", f.id,
		"execution_id", f.report.ExecutionID,
		"current_state", currentState, "current_outcome", currentOutcome,
	)
	return &FinalizeResolution{
		HTTPStatus: 200,
		Body: fiber.Map{
			"execution_state":      string(currentState),
			"verification_outcome": string(currentOutcome),
			"outcome":              string(f.outcome),
			"idempotent":           true,
		},
	}, nil
}
