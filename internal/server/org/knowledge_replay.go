package org

import (
	"database/sql"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// Knowledge OS Operator Replay handlers.
//
// Routes mounted under /api/org/knowledge/* by [Routes]. Read-only:
// retrieval / outcome events are recorded by the agent runtime, not
// by these handlers. The Replay UI ([frontend/src/modules/knowledge/])
// consumes these endpoints; their wire shapes are defined in
// internal/store/knowledge_replay.go and MUST match the UI types.
//
// Authorization: every method reads the caller's org_id from the
// tenant_ready middleware-populated c.Locals â€” there is no way to
// pass an arbitrary orgID. A user in org A always sees org A's
// events, never another tenant's, regardless of headers.

// listKnowledgeEvents handles GET /api/org/knowledge/events.
//
// Query params:
//   - limit  (optional, default 25, max 100): page size
//   - before (optional): pagination cursor â€” pass the smallest
//     occurred_at from the previous page
func (h *Handler) listKnowledgeEvents(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "25"))
	before := c.Query("before", "")

	events, err := h.deps.DB.Knowledge().ListReplayEventsForOrg(c.Context(), orgID, before, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Pagination cursor: smallest occurred_at in this page. UI passes
	// it back as ?before= to fetch the next page. Empty when no more.
	nextCursor := ""
	if len(events) > 0 && len(events) >= limit {
		nextCursor = events[len(events)-1].OccurredAt
	}
	return c.JSON(fiber.Map{
		"events":      events,
		"next_before": nextCursor,
	})
}

// getKnowledgeEvent handles GET /api/org/knowledge/events/:retrieval_id.
//
// Returns the full retrieval event including its trace, budget, and
// (if any) outcome. 404 when the retrieval does not exist or belongs
// to another org â€” these are observably identical to prevent
// cross-tenant probing.
func (h *Handler) getKnowledgeEvent(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	retrievalID := c.Params("retrieval_id")
	if retrievalID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "retrieval_id required"})
	}
	ev, err := h.deps.DB.Knowledge().GetReplayEvent(c.Context(), orgID, retrievalID)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"event": ev})
}

// listSourceSyncs handles GET /api/org/knowledge/sources/:source_id/syncs.
//
// Returns the recent sync history for one source. The Sources panel
// uses this to populate the per-card "sync history" drawer.
func (h *Handler) listSourceSyncs(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	// The source_id parameter is currently unused by the store query
	// (ListRecentSyncsForOrg returns org-wide history). When per-
	// source sync filtering is added to the store layer, this handler
	// passes the id verbatim â€” capturing it here keeps the URL
	// shape stable across that change.
	_ = c.Params("source_id")
	limit, _ := strconv.Atoi(c.Query("limit", "25"))

	syncs, err := h.deps.DB.Knowledge().ListRecentSyncsForOrg(c.Context(), orgID, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"syncs": syncs})
}

// getKnowledgeStats handles GET /api/org/knowledge/stats.
//
// Returns the headline metrics powering the Replay dashboard
// summary: asset state counts, 30-day retrieval/conversion sums,
// top-retrieved and low-conversion asset lists, stale-asset count.
func (h *Handler) getKnowledgeStats(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	stats, err := h.deps.DB.Knowledge().GetStatsForOrg(c.Context(), orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"stats": stats})
}

// getKnowledgeSoak handles GET /api/org/knowledge/soak.
//
// PR-4 (Production Soak) dashboard data. Aggregates the last
// `window_hours` hours of retrieval events into hit-rate / fallback-
// rate / semantic-score / budget drop / compliance-block / embedding-
// drift metrics. The Replay UI's soak panel reads this directly.
//
// window_hours defaults to 24. Larger windows ARE allowed (max 720
// hours / 30 days) but at MVP scale the rolling aggregate is cheap.
func (h *Handler) getKnowledgeSoak(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	if orgID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "workspace context required"})
	}
	windowHours := c.QueryInt("window_hours", 24)
	if windowHours <= 0 || windowHours > 720 {
		windowHours = 24
	}
	metrics, err := h.deps.DB.Knowledge().GetSoakMetricsForOrg(c.Context(), orgID, windowHours)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"soak": metrics})
}
