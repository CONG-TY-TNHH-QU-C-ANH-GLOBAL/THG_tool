package agent

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
	"github.com/thg/scraper/internal/server/system"
	"github.com/thg/scraper/internal/store"
)

// proofToEvidence adapts the runtime verifier's proof shape onto the
// store's evidence shape. Two types exist (instead of one shared) to
// avoid an import cycle: runtime cannot import store, and store cannot
// import runtime. The fields are 1:1 today; if they diverge, this is
// the seam to translate.
func proofToEvidence(p runtime.VerifierProof) store.VerificationEvidence {
	return store.VerificationEvidence{
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
	_ = h.db.ResetStaleSendingOutboundForOrg(orgID, 10*time.Minute)
	candidates, err := h.db.GetOutboundByStatusForOrg(orgID, string(models.OutboundApproved), limit*4)
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
		ownsStream, err := h.db.ConnectorOwnsAccountStream(orgID, agentID, msg.AccountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ownsStream {
			continue
		}
		if err := h.db.ClaimApprovedOutboundForOrg(orgID, msg.ID, workerID); err != nil {
			continue
		}
		msg.Status = models.OutboundSending
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

	outcome, attemptID := h.recordExecutionAttempt(c, orgID, id, report)

	// Translate verified outcome → outbound terminal status. Success-class
	// outcomes (dom_verified / optimistic_success / duplicate_blocked)
	// stay marked Sent; failure-class outcomes flip to Failed even though
	// the extension hit the /sent endpoint. The verifier's classification
	// supersedes the endpoint name.
	terminalStatus := models.OutboundSent
	if !models.IsSuccessOutcome(outcome) {
		terminalStatus = models.OutboundFailed
	}
	if err := h.db.UpdateOutboundStatusForOrg(orgID, id, terminalStatus); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}

	// Inbox-specific thread bookkeeping retained from the legacy flow —
	// only do it when we actually believe the message landed.
	if models.IsSuccessOutcome(outcome) {
		if msg, err := h.db.GetOutboundForOrg(orgID, id); err == nil && msg.Type == "inbox" && msg.TargetURL != "" {
			threadID, threadErr := h.db.CreateThreadForOrg(orgID, 0, string(msg.Platform), msg.TargetURL, msg.TargetName, "")
			if threadErr == nil {
				_ = h.db.AddThreadMessage(threadID, "outbound", msg.Content, true)
			}
		}
	}

	if terminalStatus == models.OutboundSent {
		system.NotifyOutboundStatus(h.db, h.notifier, orgID, id, models.OutboundSent)
	} else {
		system.NotifyOutboundStatusDetail(h.db, h.notifier, orgID, id, models.OutboundFailed, string(outcome))
	}

	return c.JSON(fiber.Map{
		"status":     string(terminalStatus),
		"outcome":    string(outcome),
		"attempt_id": attemptID,
	})
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

	outcome, attemptID := h.recordExecutionAttempt(c, orgID, id, report)

	if err := h.db.UpdateOutboundStatusForOrg(orgID, id, models.OutboundFailed); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	system.NotifyOutboundStatusDetail(h.db, h.notifier, orgID, id, models.OutboundFailed, string(outcome))
	return c.JSON(fiber.Map{
		"status":     "failed",
		"outcome":    string(outcome),
		"attempt_id": attemptID,
	})
}

// recordExecutionAttempt is the single write-point Step 3 commits the
// verifier's classification through:
//
//  1. Classify the extension report → ExecutionOutcome + VerifierProof.
//  2. Open an execution_attempts row.
//  3. Finish that row with the outcome + evidence.
//  4. Propagate the outcome to action_ledger by outbound_id.
//  5. Apply the corresponding risk signal to the executing account.
//
// Every downstream consumer (ledger, badge, risk_score, future orchestrator)
// derives its view of reality from these writes. Errors here are logged
// but never propagated to the HTTP response — the user-visible status
// change still happens; verification telemetry is best-effort, not blocking.
func (h *Handler) recordExecutionAttempt(c *fiber.Ctx, orgID, outboundID int64, report runtime.ExtensionExecutionReport) (models.ExecutionOutcome, int64) {
	ctx := c.UserContext()
	outcome, proof := runtime.ClassifyExtensionReport(report)

	msg, msgErr := h.db.GetOutboundForOrg(orgID, outboundID)
	if msgErr != nil {
		slog.WarnContext(ctx, "exec-verify: outbound lookup failed",
			"org_id", orgID, "outbound_id", outboundID, "error", msgErr)
		return outcome, 0
	}

	attemptID, err := h.db.BeginExecutionAttempt(ctx, models.ExecutionAttempt{
		OrgID:      orgID,
		OutboundID: outboundID,
		AccountID:  msg.AccountID,
		TargetURL:  msg.TargetURL,
		ActionType: msg.Type,
		Attempt:    1,
		Status:     models.AttemptVerifying,
	})
	if err != nil {
		slog.WarnContext(ctx, "exec-verify: begin attempt failed",
			"org_id", orgID, "outbound_id", outboundID, "error", err)
		return outcome, 0
	}

	failureReason := ""
	if !models.IsSuccessOutcome(outcome) {
		failureReason = report.FailureReason
		if failureReason == "" {
			failureReason = string(outcome)
		}
	}
	if err := h.db.FinishExecutionAttempt(ctx, attemptID, outcome, failureReason, proofToEvidence(proof)); err != nil {
		slog.WarnContext(ctx, "exec-verify: finish attempt failed",
			"attempt_id", attemptID, "outcome", outcome, "error", err)
	}

	// Action ledger — supersede the queued state with verified reality.
	ledgerOutcome := models.LedgerOutcomeAlias(outcome)
	ledgerReason := string(outcome)
	if failureReason != "" && failureReason != string(outcome) {
		ledgerReason = string(outcome) + ":" + failureReason
	}
	if _, err := h.db.MarkActionLedgerOutcomeByOutbound(ctx, orgID, outboundID, ledgerOutcome, ledgerReason); err != nil {
		slog.WarnContext(ctx, "exec-verify: ledger outcome update failed",
			"org_id", orgID, "outbound_id", outboundID, "error", err)
	}

	// Risk signal — only emit when the outcome maps to a meaningful
	// signal. Empty signal = ambiguous outcome (optimistic_success,
	// soft_fail, verification_timeout); we deliberately do NOT move
	// risk_score in either direction for those.
	if sig := models.RiskSignalForOutcome(outcome); sig != "" && msg.AccountID > 0 {
		if err := h.db.ApplyRiskSignal(ctx, orgID, msg.AccountID, sig, 0); err != nil {
			slog.WarnContext(ctx, "exec-verify: apply risk signal failed",
				"org_id", orgID, "account_id", msg.AccountID, "signal", sig, "error", err)
		}
	}

	slog.InfoContext(ctx, "exec-verify: attempt classified",
		"event", "execution.verified",
		"outbound_id", outboundID,
		"attempt_id", attemptID,
		"outcome", outcome,
		"account_id", msg.AccountID,
		"action_type", msg.Type,
	)
	return outcome, attemptID
}
