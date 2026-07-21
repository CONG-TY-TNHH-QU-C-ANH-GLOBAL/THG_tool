package outbox

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

// Manual human verification + retry endpoints (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md
// companion, Part A/B). JWT-authed dashboard routes; tenant + account-owner scoped via
// requireOutboundOwnerRow. Both APPEND through the canonical paths — never mutate old rows.

// resolveOwnedOutbound parses the actor locals + :id param and loads the outbound
// row the caller owns. On any failure it has already written the 400/403/404
// response and returns a non-nil error the handler should propagate. Shared by the
// comment human-verify + retry handlers (identical entry contract).
func (h *Handler) resolveOwnedOutbound(c *fiber.Ctx) (*models.OutboundMessage, int64, int64, error) {
	orgID := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)
	role, _ := c.Locals("user_role").(string)
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, 0, 0, c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	msg, ownErr := h.requireOutboundOwnerRow(c, orgID, userID, role, id)
	if ownErr != nil {
		return nil, 0, 0, ownErr // response already written
	}
	return msg, orgID, userID, nil
}

// humanVerifyComment handles POST /api/comments/:id/human-verify — an operator confirms a
// submitted_unverified comment they verified by eye on Facebook. Appends a
// 'succeeded'/'human_verified' correction (idempotent).
func (h *Handler) humanVerifyComment(c *fiber.Ctx) error {
	msg, orgID, userID, err := h.resolveOwnedOutbound(c)
	if err != nil {
		return err
	}
	if ok, reason := models.HumanVerifyEligible(*msg); !ok {
		return c.Status(400).JSON(fiber.Map{
			"error":  "not_eligible_for_manual_verify",
			"reason": reason,
		})
	}
	res, err := h.db.Coordination().AppendHumanVerifyCorrection(c.UserContext(), coordination.HumanVerifyInput{
		OrgID:           orgID,
		OutboundID:      msg.ID,
		TargetURL:       msg.TargetURL,
		AccountID:       msg.AccountID,
		VerifiedBy:      userID,
		PreviousOutcome: string(msg.VerificationOutcome),
	})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"ok":                    true,
		"outbound_id":           msg.ID,
		"corrected":             res.Corrected,
		"already_verified":      res.AlreadyVerified,
		"correction_event_id":   res.CorrectionLedgerID,
		"audit_id":              res.AuditID,
		"reason":                models.LedgerReasonHumanVerified,
		"new_effective_outcome": res.NewEffectiveOutcome,
	})
}

// retryComment handles POST /api/comments/:id/retry — re-queue a retryable PRE-SUBMIT
// failure (target_not_reached / execution_failed incl. comment_button_not_found) as a FRESH
// attempt through the canonical queue (PolicyGate / dedup / readiness). The old failed row is
// never touched; a new outbound + ledger row is appended.
func (h *Handler) retryComment(c *fiber.Ctx) error {
	msg, orgID, userID, err := h.resolveOwnedOutbound(c)
	if err != nil {
		return err
	}
	if msg.Type != "comment" || !models.IsRetryableVerificationOutcome(msg.VerificationOutcome) {
		return c.Status(400).JSON(fiber.Map{
			"error":  "not_retryable",
			"reason": string(msg.VerificationOutcome),
		})
	}
	// Re-queue through the canonical write path — same target/content/account, new attempt.
	res, err := h.db.Outbound().Queue(&models.OutboundMessage{
		OrgID:      orgID,
		Type:       "comment",
		Platform:   models.PlatformFacebook,
		AccountID:  msg.AccountID,
		TargetURL:  msg.TargetURL,
		TargetName: msg.TargetName,
		Content:    msg.Content,
		Context:    msg.Context,
		CreatedBy:  userID,
	}, 24*time.Hour)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !res.Decision.Allowed {
		return c.Status(409).JSON(fiber.Map{
			"error":       "retry_blocked",
			"reason":      res.Decision.Reason,
			"existing_id": res.Decision.ExistingID,
		})
	}
	return c.JSON(fiber.Map{"ok": true, "new_outbound_id": res.ID, "execution_state": string(res.ExecutionState)})
}

// commentOutcomeMetrics handles GET /api/comments/metrics?days=N — the outcome summary used
// to decide whether submitted_unverified is frequent enough to reopen async reverify.
func (h *Handler) commentOutcomeMetrics(c *fiber.Ctx) error {
	orgID := c.Locals("org_id").(int64)
	days := c.QueryInt("days", 7)
	if days <= 0 {
		days = 7
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	m, err := h.db.Coordination().CommentOutcomeMetrics(c.UserContext(), orgID, since)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"days":                      days,
		"metrics":                   m,
		"effective_verified":        m.EffectiveVerified(),
		"submitted_unverified_open": m.SubmittedUnverifiedOpen(),
		"submitted_unverified_rate": m.SubmittedUnverifiedRate(),
	})
}
