package outbox

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/fburl"
)

// Layer C — browser-side pre-submit verification (P1.3C).
// POST /api/agent/outbox/:id/pre-submit-verify  (and /api/connectors/outbox/:id/...)
//
// The executor calls this AFTER navigating to the target post but BEFORE typing/clicking
// Send. The backend zero-trusts the live browser context (current post id / group / URL)
// against the queued outbound target and returns {allowed, reason}. On allowed=false the
// executor MUST abort the submit and report failure via the existing /failed callback with
// the returned reason — so a wrong-post comment is never posted in the first place.
//
// This handler is a pure VERIFICATION ORACLE: it does not mutate outbound/workflow state
// (the terminal transition stays owned by the /failed callback), which keeps the new
// surface minimal and side-effect-free. It is additive and backward compatible — executors
// that do not yet call it fall back to the existing post-submit EnforceTargetIdentity check.
func (h *Handler) agentOutboxPreSubmitVerify(c *fiber.Ctx) error {
	orgID, _ := c.Locals("agent_org_id").(int64)
	if orgID <= 0 {
		return c.Status(403).JSON(fiber.Map{"error": "agent is not scoped to an organization"})
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		ExecutionID     string `json:"execution_id"`
		CurrentPostFBID string `json:"current_post_fbid"`
		CurrentStoryID  string `json:"current_story_fbid"`
		CurrentGroupRef string `json:"current_group_ref"`
		CurrentURL      string `json:"current_url"`
		AuthorSnippet   string `json:"author_snippet"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	msg, msgErr := h.db.Outbound().Get(orgID, id)
	if msgErr != nil {
		return c.Status(404).JSON(fiber.Map{"error": "outbound message not found"})
	}
	// execution_id must match the live claim (defense against a stale/duplicate executor
	// verifying a row it no longer owns). Empty stored id = legacy row → skip the check.
	if msg.ExecutionID != "" && strings.TrimSpace(body.ExecutionID) != "" && msg.ExecutionID != strings.TrimSpace(body.ExecutionID) {
		return c.Status(409).JSON(fiber.Map{"allowed": false, "reason": "stale_execution_id"})
	}

	target := directpost.ExpectedTarget{
		PostFBID:     fburl.ExtractFacebookPostID(msg.TargetURL),
		GroupRef:     fburl.ExtractGroupRef(msg.TargetURL),
		CanonicalURL: msg.TargetURL,
	}
	livePID := strings.TrimSpace(body.CurrentPostFBID)
	if livePID == "" {
		livePID = strings.TrimSpace(body.CurrentStoryID)
	}
	live := directpost.BrowserContext{
		CurrentPostFBID: livePID,
		CurrentGroupRef: strings.TrimSpace(body.CurrentGroupRef),
		CurrentURL:      strings.TrimSpace(body.CurrentURL),
		AuthorSnippet:   strings.TrimSpace(body.AuthorSnippet),
	}
	allowed, reason := directpost.VerifyBrowserContext(target, live)

	slog.InfoContext(c.UserContext(), "outbox pre-submit verify",
		"org_id", orgID, "outbound_id", id, "type", msg.Type,
		"target_url", msg.TargetURL, "expected_post_fbid", target.PostFBID, "expected_group_ref", target.GroupRef,
		"live_post_fbid", live.CurrentPostFBID, "live_group_ref", live.CurrentGroupRef, "live_url", live.CurrentURL,
		"allowed", allowed, "reason", reason,
	)
	return c.JSON(fiber.Map{"allowed": allowed, "reason": reason})
}
