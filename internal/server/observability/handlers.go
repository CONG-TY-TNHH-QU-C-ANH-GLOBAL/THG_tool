package observability

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// executionDistribution serves GET /api/observability/execution/distribution.
// Returns counts of execution_attempts grouped by (outcome, action_type)
// for the requesting org over the last N hours. The dashboard renders
// this as a matrix / stacked-bar — answers "how is the platform treating
// our actions right now?" without scanning individual rows.
//
// Response shape: { window_hours, since, buckets: [{outcome, action_type, count}], total }
func executionDistribution(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, _ := strconv.Atoi(c.Query("hours", "24"))
		if hours <= 0 {
			hours = 24
		}
		if hours > 168 {
			hours = 168
		}
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

		buckets, err := deps.DB.ExecutionOutcomeDistribution(c.UserContext(), orgID, since)
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

// recentAttemptRow is the wire shape served by /recent. Same fields as
// models.ExecutionAttempt plus a parsed `evidence` object so the
// dashboard doesn't have to JSON.parse the embedded string client-side.
type recentAttemptRow struct {
	ID             int64                  `json:"id"`
	ActionLedgerID int64                  `json:"action_ledger_id"`
	OutboundID     int64                  `json:"outbound_id"`
	OrgID          int64                  `json:"org_id"`
	AccountID      int64                  `json:"account_id"`
	TargetURL      string                 `json:"target_url"`
	ActionType     string                 `json:"action_type"`
	Attempt        int                    `json:"attempt"`
	Status         string                 `json:"status"`
	Outcome        string                 `json:"outcome"`
	FailureReason  string                 `json:"failure_reason"`
	Evidence       map[string]any         `json:"evidence,omitempty"`
	DOMVerified    bool                   `json:"dom_verified"`
	StartedAt      string                 `json:"started_at"`
	FinishedAt     string                 `json:"finished_at,omitempty"`
}

// executionRecent serves GET /api/observability/execution/recent.
// Returns the most recent execution_attempts rows for the org, newest
// first. Default 100 rows, max 500. The `evidence_json` column is parsed
// server-side into a structured object so the dashboard renders proof
// fields (comment_permalink / message_bubble_id / page_url_after /
// notes) without client-side JSON parsing.
//
// This is the canonical "what just happened" feed — a row per attempt,
// each carrying both the outcome AND the proof. Operators read it the
// way an SRE reads tail-of-log: scroll, spot anomalies, dig deeper.
func executionRecent(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		hours, _ := strconv.Atoi(c.Query("hours", "24"))
		if hours <= 0 {
			hours = 24
		}
		if hours > 168 {
			hours = 168
		}
		limit, _ := strconv.Atoi(c.Query("limit", "100"))
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

		attempts, err := deps.DB.ListRecentExecutionAttempts(c.UserContext(), orgID, since, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]recentAttemptRow, 0, len(attempts))
		for _, a := range attempts {
			row := recentAttemptRow{
				ID:             a.ID,
				ActionLedgerID: a.ActionLedgerID,
				OutboundID:     a.OutboundID,
				OrgID:          a.OrgID,
				AccountID:      a.AccountID,
				TargetURL:      a.TargetURL,
				ActionType:     a.ActionType,
				Attempt:        a.Attempt,
				Status:         string(a.Status),
				Outcome:        string(a.Outcome),
				FailureReason:  a.FailureReason,
				DOMVerified:    a.DOMVerified,
				StartedAt:      a.StartedAt.Format(time.RFC3339),
			}
			if !a.FinishedAt.IsZero() {
				row.FinishedAt = a.FinishedAt.Format(time.RFC3339)
			}
			if strings.TrimSpace(a.EvidenceJSON) != "" {
				var ev map[string]any
				if err := json.Unmarshal([]byte(a.EvidenceJSON), &ev); err == nil && len(ev) > 0 {
					row.Evidence = ev
				}
			}
			out = append(out, row)
		}
		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"attempts":     out,
			"count":        len(out),
		})
	}
}

// executionAccountHealth serves GET /api/observability/execution/account-health.
// Per-account behaviour-profile + runtime snapshot. Pair with /distribution
// to answer "is this account being soft-blocked?" — risk_score going up
// while outcomes show shadow_rejected / rate_limited concentrations on
// the same account is the canonical poisoned-account signature.
//
// Optional ?account_id= filters to a single account. Default: every
// account in the org, ordered by risk_score DESC (worst first).
//
// Caveat for the dashboard: TrustLevel="" means "no behaviour_profile
// row exists yet for this account" — render as "default warming" per the
// policy resolver's NormalizeTrustLevel fallback.
func executionAccountHealth(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": "tenant not scoped"})
		}
		accountID, _ := strconv.ParseInt(c.Query("account_id", "0"), 10, 64)

		rows, err := deps.DB.AccountHealthSnapshot(c.UserContext(), orgID, accountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// Normalize the trust_level fallback here so the dashboard doesn't
		// need to know the policy resolver's default. Empty string in the
		// row → "warming" on the wire.
		type wireRow struct {
			store.AccountHealthRow
			TrustLevel string `json:"trust_level"`
		}
		out := make([]wireRow, 0, len(rows))
		for _, r := range rows {
			trust := strings.TrimSpace(r.TrustLevel)
			if trust == "" {
				trust = string(models.TrustWarming)
			}
			out = append(out, wireRow{AccountHealthRow: r, TrustLevel: trust})
		}
		return c.JSON(fiber.Map{
			"accounts": out,
			"count":    len(out),
		})
	}
}
