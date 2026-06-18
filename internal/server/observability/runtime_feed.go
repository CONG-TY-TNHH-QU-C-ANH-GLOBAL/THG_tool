package observability

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store/coordination"
)

// runtimeFeedRow is the wire shape the dashboard renders. attrs_json
// is parsed server-side so the FE doesn't have to JSON.parse rows in
// a tight render loop.
type runtimeFeedRow struct {
	ID         int64          `json:"id"`
	OrgID      int64          `json:"org_id"`
	AccountID  int64          `json:"account_id"`
	Event      string         `json:"event"`
	Level      string         `json:"level"`
	OutboundID int64          `json:"outbound_id"`
	AttemptID  int64          `json:"attempt_id"`
	TargetURL  string         `json:"target_url,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

// buildRuntimeFeedRow maps one runtime_events record to its wire row, parsing
// attrs_json server-side. Extracted verbatim from runtimeFeed's loop body so
// the handler keeps only the level/event filtering; behavior (fields, format,
// attrs parsing) is unchanged.
func buildRuntimeFeedRow(r coordination.RuntimeEvent) runtimeFeedRow {
	row := runtimeFeedRow{
		ID:         r.ID,
		OrgID:      r.OrgID,
		AccountID:  r.AccountID,
		Event:      r.Event,
		Level:      r.Level,
		OutboundID: r.OutboundID,
		AttemptID:  r.AttemptID,
		TargetURL:  r.TargetURL,
		CreatedAt:  r.CreatedAt.Format(time.RFC3339),
	}
	if r.AttrsJSON != "" && r.AttrsJSON != "{}" {
		var attrs map[string]any
		if err := json.Unmarshal([]byte(r.AttrsJSON), &attrs); err == nil && len(attrs) > 0 {
			row.Attrs = attrs
		}
	}
	return row
}

// runtimeFeed serves GET /api/observability/runtime-feed.
// Returns the typed runtime event tail for the requesting org over a
// bounded time window. Pairs with the events package (typed taxonomy)
// and the runtime_events persistence table.
//
// Response: { window_hours, since, events: [...], count }
//
// Query params:
//   - hours       (default 1, max 168) — window size
//   - limit       (default 100, max 500) — row cap
//   - level       (optional: info | warn) — filter by severity
//   - event       (optional, exact match) — filter by event name
//
// The level/event query filters are applied client-side (post-fetch)
// to keep the store query simple. The store query itself is bounded
// by orgID + time window + limit only.
func runtimeFeed(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, _ := strconv.Atoi(c.Query("hours", "1"))
		if hours <= 0 {
			hours = 1
		}
		if hours > 168 {
			hours = 168
		}
		limit, _ := strconv.Atoi(c.Query("limit", "100"))
		levelFilter := c.Query("level", "")
		eventFilter := c.Query("event", "")
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

		rows, err := deps.DB.Coordination().ListRecentRuntimeEvents(c.UserContext(), orgID, since, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		out := make([]runtimeFeedRow, 0, len(rows))
		for _, r := range rows {
			if levelFilter != "" && r.Level != levelFilter {
				continue
			}
			if eventFilter != "" && r.Event != eventFilter {
				continue
			}
			out = append(out, buildRuntimeFeedRow(r))
		}

		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"events":       out,
			"count":        len(out),
		})
	}
}
