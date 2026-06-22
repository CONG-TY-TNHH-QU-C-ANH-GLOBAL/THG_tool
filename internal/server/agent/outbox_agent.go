package agent

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime"
)

// Outbound HTTP handlers for the local-connector agent: claim approved outbound
// for execution (GET /outbox) and record the terminal /sent and /failed
// callbacks. The outbound finalization state machine these callbacks drive lives
// in outbox_finalize*.go; the claim helper in outbox_claim.go.

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
		claimed, ok, err := h.claimCandidate(orgID, agentID, assignedAccountID, workerID, msg)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if !ok {
			continue
		}
		msgs = append(msgs, claimed)
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
