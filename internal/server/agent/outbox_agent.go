package agent

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store/coordination"
)

// proofToEvidence adapts the runtime verifier's proof shape onto the
// coordination domain's evidence shape. Two types exist (instead of
// one shared) to avoid an import cycle: runtime cannot import store,
// and coordination cannot import runtime. The fields are 1:1 today;
// if they diverge, this is the seam to translate.
func proofToEvidence(p runtime.VerifierProof) coordination.VerificationEvidence {
	return coordination.VerificationEvidence{
		CommentPermalink: p.CommentPermalink,
		MessageBubbleID:  p.MessageBubbleID,
		DOMSnippet:       p.DOMSnippet,
		PageURLAfter:     p.PageURLAfter,
		ObservedAt:       p.ObservedAt,
		Notes:            p.Notes,
	}
}

// agentGetOutbox returns approved outbound messages for local execution.
// GET /api/agent/outbox
//
// Each successful claim issues a fresh execution_id and stamps a
// per-row lease_expiry (see store.DefaultOutboundLease). Both fields
// flow back in the response so the executor can echo execution_id
// on its /sent or /failed callback — that token gates the terminal
// CAS in store.FinalizeOutboundAttempt and is what prevents
// duplicate-comment when the extension's service worker restarts
// mid-execution or a flaky network triggers a retry.
func (h *Handler) agentGetOutbox(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	agentID, _ := c.Locals("agent_id").(int64)
	assignedAccountID, _ := c.Locals("agent_assigned_account_id").(int64)
	workerID, _ := c.Locals("agent_token_fp").(string)
	if orgID <= 0 || agentID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	// 10-min fallback only applies to legacy rows (lease_expiry IS
	// NULL). New claims get a per-row lease so this global window is
	// no longer the primary stale-detection knob.
	_ = h.db.ResetStaleExecutingForOrg(orgID, 10*time.Minute)
	candidates, err := h.db.GetOutboundByExecutionStateForOrg(orgID, models.ExecPlanned, "", limit*4)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	msgs := make([]models.OutboundMessage, 0, limit)
	for _, msg := range candidates {
		if len(msgs) >= limit {
			break
		}
		if msg.AccountID <= 0 {
			continue
		}
		if assignedAccountID > 0 && msg.AccountID != assignedAccountID {
			continue
		}
		ownsStream, err := h.db.Connectors().ConnectorOwnsAccountStream(orgID, agentID, msg.AccountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ownsStream {
			continue
		}
		claim, err := h.db.ClaimPlannedOutboundForOrg(orgID, msg.ID, workerID, 0)
		if err != nil || claim == nil {
			continue
		}
		msg.ExecutionState = models.ExecExecuting
		msg.ExecutionID = claim.ExecutionID
		// Activity feed: execution_started — the autonomous-first
		// vocabulary makes "extension claimed and is about to mutate
		// the live DOM" a distinct event from "intent was queued"
		// (execution_planned) and from terminal events
		// (execution_verified / execution_failed).
		system.NotifyExecutionStarted(h.db, orgID, msg.AccountID, msg.ID, claim.ExecutionID, msg.Type)
		msgs = append(msgs, msg)
	}
	return c.JSON(fiber.Map{"messages": msgs, "count": len(msgs)})
}

// agentOutboxSent records a verified send attempt.
// POST /api/agent/outbox/:id/sent
//
// Step 3 — Execution Verification. Legacy contract: hitting this endpoint
// asserts the extension thinks the action succeeded. New contract: the
// body MAY include an ExtensionExecutionReport with DOM proof
// (CommentPermalink / MessageBubbleID / DOMSnippet / PageURLAfter /
// CountIncreased / NodeMatched / BubbleFresh / Duplicate / Notes). The
// server classifies the outcome using the same taxonomy a server-side
// chromedp verifier would emit, writes an execution_attempts row, updates
// the action_ledger by outbound_id, and applies the corresponding risk
// signal. Extensions that POST with no body still mark the outbound as
// sent (OptimisticSuccess) — backward-compatible.
func (h *Handler) agentOutboxSent(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Legacy contract default: hitting /sent asserts success. The body
	// supplements with DOM evidence when present.
	report := runtime.ExtensionExecutionReport{Success: true}
	_ = c.BodyParser(&report)

	outcome, proof := runtime.ClassifyExtensionReport(report)
	// PR-1: terminal pair (state, outcome) instead of a single status.
	// The verifier's classification supersedes the endpoint name —
	// even though /sent fires, a non-success outcome lands the row in
	// finished/<non-verified> per TerminalFromOutcome.
	resolution, err := h.finalizeOutbound(c, orgID, id, report, outcome, proof)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return resolution.write(c)
}

// agentOutboxFailed records a failed send attempt.
// POST /api/agent/outbox/:id/failed
//
// Step 3 — Execution Verification. Legacy contract: hitting this endpoint
// asserts the extension failed to send. New contract: the body MAY include
// an ExtensionExecutionReport whose FailureReason maps to a specific
// outcome (captcha, rate_limited, blocked, redirected_feed, …). Without a
// body the classifier returns ExecutionShadowRejected — the safe default
// that flags the row as a real failure rather than letting it silently
// claim success.
func (h *Handler) agentOutboxFailed(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Default failure assertion; body may supply richer FailureReason.
	report := runtime.ExtensionExecutionReport{Success: false}
	_ = c.BodyParser(&report)

	outcome, proof := runtime.ClassifyExtensionReport(report)
	// Same execution_id-gated CAS pathway as agentOutboxSent.
	resolution, err := h.finalizeOutbound(c, orgID, id, report, outcome, proof)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return resolution.write(c)
}

// finalizeResolution is the HTTP-shaped result of a /sent or /failed
// callback. The handler builds one of these and writes it through.
// Centralising the shape here keeps the two handlers symmetric and
// makes the three terminal pathways (committed / idempotent replay /
// stale execution_id) easy to audit.
type finalizeResolution struct {
	HTTPStatus int
	Body       fiber.Map
}

func (f *finalizeResolution) write(c *fiber.Ctx) error {
	return c.Status(f.HTTPStatus).JSON(f.Body)
}

// finalizeOutbound is the single write-point for terminal outbound
// callbacks. It encodes three invariants:
//
//  1. EXECUTION IDENTITY (post-hoc defense for wrong-post bug class):
//     EnforceTargetIdentity downgrades success-class outcomes to
//     ContextDrift when the extension's reported page_url_after does
//     not address the same Facebook entity as the queued target_url.
//
//  2. TERMINAL CAS (lease + execution_id idempotency):
//     FinalizeOutboundAttempt requires (status='sending', execution_id
//     matches OR row's execution_id is empty for legacy rows). A
//     replayed callback hitting an already-terminal row returns
//     idempotent-OK; a callback whose execution_id no longer matches
//     the row (because ResetStaleExecutingForOrg lease-evicted
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
func (h *Handler) finalizeOutbound(
	c *fiber.Ctx,
	orgID, id int64,
	report runtime.ExtensionExecutionReport,
	outcome models.ExecutionOutcome,
	proof runtime.VerifierProof,
) (*finalizeResolution, error) {
	ctx := c.UserContext()

	msg, msgErr := h.db.GetOutboundForOrg(orgID, id)
	if msgErr != nil {
		return &finalizeResolution{
			HTTPStatus: 404,
			Body:       fiber.Map{"error": "outbound message not found"},
		}, nil
	}

	// Defense-in-depth identity check. EnforceTargetIdentity downgrades
	// success-class outcomes to ContextDrift if target_url/page_url_after
	// entity ids mismatch.
	outcome, proof = runtime.EnforceTargetIdentity(outcome, proof, msg.TargetURL, msg.Type)

	// Diagnostic instrumentation: emit a structured log line for every
	// non-success terminal so operators can see WHAT the extension did
	// without having to query execution_attempts.evidence_json. Captures
	// the proof.notes field which carries the landed_url + gate-fail
	// detail from the extension's lifecycle gates (see outbound.js patch
	// 3c17f1a). Precursor to EXP-1 typed events on the Runtime Control
	// Plane (see project_runtime_control_plane memory).
	if !models.IsSuccessOutcome(outcome) {
		slog.WarnContext(ctx, "exec-verify: non-success outcome",
			"org_id", orgID, "outbound_id", id,
			"account_id", msg.AccountID,
			"target_url", msg.TargetURL,
			"outcome", string(outcome),
			"failure_reason", report.FailureReason,
			"page_url_after", proof.PageURLAfter,
			"notes", proof.Notes,
			"dom_snippet", proof.DOMSnippet,
		)
	}

	// PR-1 dual-column terminal pair: (state, verification_outcome).
	// TerminalFromOutcome is the single mapping from the rich
	// execution_attempts taxonomy onto the outbound row's two new
	// columns. ExecExpired only applies for ExecutionRetryExhausted
	// — every other terminal carries some observation.
	terminalState, terminalOutcome := models.TerminalFromOutcome(outcome)

	finalized, currentState, currentOutcome, currentExecID, err := h.db.FinalizeOutboundAttempt(ctx, orgID, id, report.ExecutionID, terminalState, terminalOutcome)
	if err != nil {
		return nil, err
	}
	if !finalized {
		// Disambiguate: is this a replay (same token, already terminal)
		// or a stale (token mismatch, row was re-claimed)?
		if report.ExecutionID != "" && currentExecID != "" && report.ExecutionID != currentExecID {
			// Stale: the row was lease-evicted and re-claimed; this
			// callback belongs to an execution that no longer owns
			// the row. Refuse loudly — 409 surfaces to the dashboard.
			slog.WarnContext(ctx, "exec-verify: stale execution_id",
				"org_id", orgID, "outbound_id", id,
				"submitted_execution_id", report.ExecutionID,
				"current_execution_id", currentExecID,
				"current_state", currentState,
				"current_outcome", currentOutcome,
			)
			return &finalizeResolution{
				HTTPStatus: 409,
				Body: fiber.Map{
					"error":                "stale execution_id",
					"current_state":        string(currentState),
					"current_outcome":      string(currentOutcome),
					"current_execution_id": currentExecID,
				},
			}, nil
		}
		// Idempotent replay: same execution_id, row already terminal.
		// Return success-shaped response WITHOUT replaying side effects.
		slog.InfoContext(ctx, "exec-verify: idempotent replay",
			"org_id", orgID, "outbound_id", id,
			"execution_id", report.ExecutionID,
			"current_state", currentState, "current_outcome", currentOutcome,
		)
		return &finalizeResolution{
			HTTPStatus: 200,
			Body: fiber.Map{
				"execution_state":      string(currentState),
				"verification_outcome": string(currentOutcome),
				"outcome":              string(outcome),
				"idempotent":           true,
			},
		}, nil
	}

	// FIRST-WIN PATH — commit side effects exactly once.
	attemptID, err := h.db.Coordination().BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID:      orgID,
		OutboundID: id,
		AccountID:  msg.AccountID,
		TargetURL:  msg.TargetURL,
		ActionType: msg.Type,
		Attempt:    1,
		Status:     models.AttemptVerifying,
	})
	if err != nil {
		slog.WarnContext(ctx, "exec-verify: begin attempt failed",
			"org_id", orgID, "outbound_id", id, "error", err)
		attemptID = 0
	} else {
		failureReason := ""
		if !models.IsSuccessOutcome(outcome) {
			failureReason = report.FailureReason
			if failureReason == "" {
				failureReason = string(outcome)
			}
		}
		if err := h.db.Coordination().FinishExecutionAttempt(ctx, attemptID, outcome, failureReason, proofToEvidence(proof)); err != nil {
			slog.WarnContext(ctx, "exec-verify: finish attempt failed",
				"attempt_id", attemptID, "outcome", outcome, "error", err)
		}

		ledgerOutcome := models.LedgerOutcomeAlias(outcome)
		ledgerReason := string(outcome)
		if failureReason != "" && failureReason != string(outcome) {
			ledgerReason = string(outcome) + ":" + failureReason
		}
		if _, err := h.db.Coordination().MarkActionLedgerOutcomeByOutbound(ctx, orgID, id, ledgerOutcome, ledgerReason); err != nil {
			slog.WarnContext(ctx, "exec-verify: ledger outcome update failed",
				"org_id", orgID, "outbound_id", id, "error", err)
		}

		if sig := models.RiskSignalForOutcome(outcome); sig != "" && msg.AccountID > 0 {
			if err := h.db.Coordination().ApplyRiskSignal(ctx, orgID, msg.AccountID, sig, 0); err != nil {
				slog.WarnContext(ctx, "exec-verify: apply risk signal failed",
					"org_id", orgID, "account_id", msg.AccountID, "signal", sig, "error", err)
			}
		}
		slog.InfoContext(ctx, "exec-verify: attempt classified",
			"event", "execution.verified",
			"outbound_id", id,
			"attempt_id", attemptID,
			"outcome", outcome,
			"account_id", msg.AccountID,
			"action_type", msg.Type,
		)
	}

	// Inbox-specific thread bookkeeping — only on actual landing.
	if models.IsSuccessOutcome(outcome) && msg.Type == "inbox" && msg.TargetURL != "" {
		if threadID, threadErr := h.db.CreateThreadForOrg(orgID, 0, string(msg.Platform), msg.TargetURL, msg.TargetName, ""); threadErr == nil {
			_ = h.db.AddThreadMessage(threadID, "outbound", msg.Content, true)
		}
	}

	// PR-2 V2: notification now consumes the (state, outcome) pair
	// directly — no legacy OutboundStatus translation. The single-source-
	// of-truth predicate IsVerifiedSuccess gates the verified/failed event
	// fork inside NotifyOutboundStatusDetail.
	if models.IsVerifiedSuccess(terminalState, terminalOutcome) {
		system.NotifyOutboundStatus(h.db, h.notifier, orgID, id, terminalState, terminalOutcome)
	} else {
		system.NotifyOutboundStatusDetail(h.db, h.notifier, orgID, id, terminalState, terminalOutcome, string(outcome))
	}

	return &finalizeResolution{
		HTTPStatus: 200,
		Body: fiber.Map{
			"execution_state":      string(terminalState),
			"verification_outcome": string(outcome),
			"attempt_id":           attemptID,
		},
	}, nil
}
