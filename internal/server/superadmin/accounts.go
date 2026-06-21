package superadmin

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

func (h *Handler) superAdminAccounts(c *fiber.Ctx) error {
	accounts, err := h.deps.DB.Identities().GetAllAccounts(0)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// AccountSafe projection: GetAllAccounts returns DECRYPTED cookies +
	// proxy/user-agent for internal workers — never serialize the raw model.
	safe := models.AccountSafeList(accounts)
	return c.JSON(fiber.Map{"accounts": safe, "count": len(safe)})
}

func (h *Handler) superAdminDeleteAccount(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidID})
	}
	if h.deps.Workspace != nil {
		h.deps.Workspace.Stop(id)
	}
	if err := h.deps.DB.Identities().DeleteAccount(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// superAdminAccountResetRisk clears risk_score + recent_failures +
// cooldown_until AND the per-day action counters (comments_today etc.) for
// an account — i.e. a clean runtime-state slate for a fresh diagnostic test.
// Founder-only. Audit logged.
//
// Why the daily counters are included: comments_today increments at QUEUE
// time (internal/store/outbound/queue.go), so a debugging loop that queues
// many attempts — even ones that fail or are probes and post nothing —
// exhausts the daily cap and blocks further tests with daily_limit_exceeded.
// Clearing the counters here lets the operator re-test immediately instead of
// waiting for the UTC day rollover.
//
// USE WITH DISCIPLINE: resetting does NOT fix the underlying cause of past
// failures. The reset is for diagnostic loops after the root cause is fixed,
// not as a recurring patch.
func (h *Handler) superAdminAccountResetRisk(c *fiber.Ctx) error {
	accountID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || accountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid account id"})
	}
	_, err = h.deps.DB.DB().ExecContext(c.Context(),
		`UPDATE account_runtime_state
		    SET risk_score = 0,
		        recent_failures = 0,
		        cooldown_until = NULL,
		        comments_today = 0,
		        inbox_today = 0,
		        group_posts_today = 0,
		        profile_posts_today = 0
		  WHERE account_id = ?`, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	userID, _ := c.Locals("user_id").(int64)
	h.deps.DB.InsertAuditLog(userID, "account_risk_reset", c.IP(),
		`{"account_id":`+strconv.FormatInt(accountID, 10)+`}`)
	return c.JSON(fiber.Map{"ok": true, "account_id": accountID})
}
