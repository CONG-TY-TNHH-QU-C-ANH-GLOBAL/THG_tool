package observability

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Watchpoint B — Prompt Routing Observability handlers.
//
// All four endpoints are read-only, org-scoped, and bounded. They render
// the dashboard's "Prompt Routing Reality" panels:
//
//   GET /api/observability/prompt-routing/distribution    — route × reason matrix
//   GET /api/observability/prompt-routing/recent          — newest-first prompt feed
//   GET /api/observability/prompt-routing/conflicts       — heuristic-flagged rows
//   GET /api/observability/prompt-routing/missing-signals — ask-back signal-gap histogram
//
// These surfaces exist to make orchestration measurable, NOT to control
// it. No write methods, no auto-decisions, no resolver scoring.

func promptRoutingHoursWindow(c *fiber.Ctx) (int, time.Time) {
	hours, _ := strconv.Atoi(c.Query("hours", "24"))
	if hours <= 0 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}
	return hours, time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
}

// promptRoutingDistribution serves
// GET /api/observability/prompt-routing/distribution.
// Returns counts grouped by (route, reason_code, action) so the
// dashboard can render the route-share + reason-breakdown matrix.
func promptRoutingDistribution(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, since := promptRoutingHoursWindow(c)
		buckets, err := deps.DB.PromptRoutingDistribution(c.UserContext(), orgID, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		total := 0
		for _, b := range buckets {
			total += b.Count
		}
		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"buckets":      buckets,
			"total":        total,
		})
	}
}

// promptRoutingRecent serves
// GET /api/observability/prompt-routing/recent.
// Returns newest-first prompt rows with parsed routing decision so the
// dashboard can render the "what just happened" feed.
func promptRoutingRecent(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, since := promptRoutingHoursWindow(c)
		limit, _ := strconv.Atoi(c.Query("limit", "100"))
		rows, err := deps.DB.RecentPromptRouting(c.UserContext(), orgID, since, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"rows":         rows,
			"count":        len(rows),
		})
	}
}

// promptRoutingConflicts serves
// GET /api/observability/prompt-routing/conflicts.
// Returns heuristic-flagged rows (false-positive deterministic /
// false-negative deterministic) so operators can audit routing edges.
// The predicate comes from Deps.PromptIsSelfSufficient — kept here so
// observability doesn't import internal/ai directly.
func promptRoutingConflicts(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, since := promptRoutingHoursWindow(c)
		conflicts, err := deps.DB.PromptRoutingConflictCandidates(c.UserContext(), orgID, since, deps.PromptIsSelfSufficient)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// Split by kind for the dashboard to render two tables.
		fp := make([]any, 0)
		fn := make([]any, 0)
		for _, conf := range conflicts {
			switch conf.Kind {
			case "false_positive_deterministic":
				fp = append(fp, conf)
			case "false_negative_deterministic":
				fn = append(fn, conf)
			}
		}
		return c.JSON(fiber.Map{
			"window_hours":            hours,
			"since":                   since.Format(time.RFC3339),
			"false_positive_count":    len(fp),
			"false_negative_count":    len(fn),
			"false_positive_examples": fp,
			"false_negative_examples": fn,
		})
	}
}

// promptRoutingMissingSignals serves
// GET /api/observability/prompt-routing/missing-signals.
// Returns the histogram of which operational signals were missing on
// ask-back rows. Renders as the dashboard's "Ambiguous Prompt Surface"
// — answers "which signal do users keep forgetting to specify?".
func promptRoutingMissingSignals(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, since := promptRoutingHoursWindow(c)
		buckets, err := deps.DB.MissingSignalDistribution(c.UserContext(), orgID, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"buckets":      buckets,
		})
	}
}
