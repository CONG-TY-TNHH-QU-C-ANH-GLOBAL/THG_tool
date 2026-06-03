package autoflow

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// autoflowContributionLeaderboard returns the derived per-member contribution
// leaderboard (Organic Sales Network Attribution Layer): a projection over the
// action_ledger keyed by the IMMUTABLE created_by member — NOT account ownership.
// The top row is the org "champion" (analytics only; no routing/execution
// priority, no lead ownership). Query: ?days=N (default 30, 0=all-time), ?limit=N.
func (h *Handler) autoflowContributionLeaderboard(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	days, _ := strconv.Atoi(c.Query("days", "30"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	var since time.Time
	if days > 0 {
		since = time.Now().UTC().AddDate(0, 0, -days)
	}
	rows, err := h.deps.DB.Coordination().ContributionLeaderboard(c.Context(), orgID, since, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	champion := ""
	if len(rows) > 0 {
		champion = rows[0].UserName
	}
	return c.JSON(fiber.Map{"leaderboard": rows, "champion": champion, "count": len(rows)})
}
