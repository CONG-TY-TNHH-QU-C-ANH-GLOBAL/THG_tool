package outbox

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/runtime/events"
)

// Async comment reverify agent endpoints (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A).
// The connector polls /reverify/claim for posts to re-check, re-opens each, searches the
// DOM for the comment by actor + normalized text, and reports the verdict to
// /reverify/result. A positive match makes the backend append the append-only correction.
// Agent-token authed; scoped to the connector's org + assigned account.

// agentReverifyClaim handles GET /api/agent/reverify/claim?limit=N.
func (h *Handler) agentReverifyClaim(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	accountID, _ := c.Locals("agent_assigned_account_id").(int64)
	tokenID, _ := c.Locals("agent_id").(int64) // WHO is claiming — recorded for diagnosis
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	limit := c.QueryInt("limit", 5)
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	jobs, err := h.db.Coordination().ClaimDueReverifies(c.Context(), orgID, accountID, tokenID, time.Now(), limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if len(jobs) > 0 {
		events.Info(c.Context(), events.ReverifyClaim,
			events.FieldOrgID, orgID, events.FieldAccountID, accountID,
			"token_id", tokenID, "count", len(jobs))
	}
	return c.JSON(fiber.Map{"reverifies": jobs, "count": len(jobs)})
}

// agentReverifyResult handles POST /api/agent/reverify/result with body
// {"id": N, "found": bool, "comment_permalink": "...", "notes": "..."}.
func (h *Handler) agentReverifyResult(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	var body struct {
		ID               int64  `json:"id"`
		Found            bool   `json:"found"`
		CommentPermalink string `json:"comment_permalink"`
		Notes            string `json:"notes"`
		Error            string `json:"error"`
	}
	if err := c.BodyParser(&body); err != nil || body.ID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "id required"})
	}
	// The connector ALWAYS reports a terminal verdict for a claimed job. An `error` (could
	// not navigate / reach the content script / post) records outcome=error so the job never
	// sits pending forever. Otherwise found→correction / not-found.
	co := h.db.Coordination()
	var corrected bool
	var err error
	outcome := "verified"
	if e := body.Error; e != "" {
		outcome = "error"
		err = co.RecordReverifyError(c.Context(), orgID, body.ID, e)
	} else if body.Found {
		corrected, err = co.ApplyReverifyResult(c.Context(), orgID, body.ID, true, body.CommentPermalink, body.Notes)
	} else {
		outcome = "not_found"
		_, err = co.ApplyReverifyResult(c.Context(), orgID, body.ID, false, "", body.Notes)
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	events.Info(c.Context(), events.ReverifyResult,
		events.FieldOrgID, orgID, "reverify_id", body.ID, events.FieldOutcome, outcome,
		"corrected", corrected, events.FieldReason, body.Error)
	return c.JSON(fiber.Map{"ok": true, "corrected": corrected, "outcome": outcome})
}
