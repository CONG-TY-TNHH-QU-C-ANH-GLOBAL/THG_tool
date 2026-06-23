package observability

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/coordination"
)

// Transport-layer error message, factored out to avoid a duplicated string
// literal (go:S1192) across this package's handlers. Value is byte-identical.
const errTenantNotScoped = "tenant not scoped"

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
			return c.Status(403).JSON(fiber.Map{"error": errTenantNotScoped})
		}
		hours, _ := strconv.Atoi(c.Query("hours", "24"))
		if hours <= 0 {
			hours = 24
		}
		if hours > 168 {
			hours = 168
		}
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

		buckets, err := deps.DB.Coordination().ExecutionOutcomeDistribution(c.UserContext(), orgID, since)
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
	ID             int64          `json:"id"`
	ActionLedgerID int64          `json:"action_ledger_id"`
	OutboundID     int64          `json:"outbound_id"`
	OrgID          int64          `json:"org_id"`
	AccountID      int64          `json:"account_id"`
	TargetURL      string         `json:"target_url"`
	ActionType     string         `json:"action_type"`
	Attempt        int            `json:"attempt"`
	Status         string         `json:"status"`
	Outcome        string         `json:"outcome"`
	FailureReason  string         `json:"failure_reason"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	DOMVerified    bool           `json:"dom_verified"`
	StartedAt      string         `json:"started_at"`
	FinishedAt     string         `json:"finished_at,omitempty"`
}

// buildRecentAttemptRow maps one execution_attempts record to its wire row,
// parsing the embedded evidence_json into a structured object server-side.
// Extracted verbatim from executionRecent's loop body so the handler stays a
// thin fetch→map→respond; behavior (fields, formats, evidence parsing) is
// unchanged.
func buildRecentAttemptRow(a models.ExecutionAttempt) recentAttemptRow {
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
	return row
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
			return c.Status(403).JSON(fiber.Map{"error": errTenantNotScoped})
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

		attempts, err := deps.DB.Coordination().ListRecentExecutionAttempts(c.UserContext(), orgID, since, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		out := make([]recentAttemptRow, 0, len(attempts))
		for _, a := range attempts {
			out = append(out, buildRecentAttemptRow(a))
		}
		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"attempts":     out,
			"count":        len(out),
		})
	}
}

// executionGapDetection serves GET /api/observability/execution/gap-detection.
// Surfaces outbound_messages stuck in planned/executing with no matching
// execution_attempts row — the executor crashed before BeginAttempt or
// the lease holder vanished. Operators see "leads queued but never
// executed" without writing SQL.
//
// Default threshold = 10 min (anything younger than that is plausibly
// still-in-flight). Override with ?older_than_minutes=N (clamped 1..1440).
// Default limit = 50, max 500.
//
// Response: { older_than_minutes, threshold, rows: [...], count }
func executionGapDetection(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": errTenantNotScoped})
		}
		minutes, _ := strconv.Atoi(c.Query("older_than_minutes", "10"))
		if minutes < 1 {
			minutes = 10
		}
		if minutes > 1440 {
			minutes = 1440
		}
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		threshold := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute)

		stuck, err := deps.DB.Coordination().GapDetection(c.UserContext(), orgID, threshold, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"older_than_minutes": minutes,
			"threshold":          threshold.Format(time.RFC3339),
			"rows":               stuck,
			"count":              len(stuck),
		})
	}
}

// executionAccountTimeseries serves
// GET /api/observability/execution/account-timeseries?account_id=X&hours=72.
// Returns hourly-bucketed outcome counts for one account over the window.
// Pairs with /account-health: the snapshot says "this account is poisoned
// right now", the timeseries shows "since when, and what's the outcome
// shape over time."
//
// Required: account_id > 0. Default hours=72, max 168.
func executionAccountTimeseries(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": errTenantNotScoped})
		}
		accountID, _ := strconv.ParseInt(c.Query("account_id", "0"), 10, 64)
		if accountID <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "account_id required"})
		}
		hours, _ := strconv.Atoi(c.Query("hours", "72"))
		if hours <= 0 {
			hours = 72
		}
		if hours > 168 {
			hours = 168
		}
		since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

		buckets, err := deps.DB.Coordination().AccountOutcomeTimeseries(c.UserContext(), orgID, accountID, since)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"account_id":   accountID,
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"buckets":      buckets,
			"count":        len(buckets),
		})
	}
}

// executionLedgerReconcile serves
// GET /api/observability/execution/ledger-reconcile?hours=24.
// Surfaces action_ledger rows where ledger.outcome='succeeded' but the
// latest execution_attempts.outcome for the same outbound_id is in a
// failure-class bucket — i.e. the ledger hallucinated success. This is
// the badge/risk/orchestrator corruption pathway warned about in
// project_execution_verification.
//
// Default hours=24, max 168. Default limit=100, max 500.
func executionLedgerReconcile(deps Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID <= 0 {
			return c.Status(403).JSON(fiber.Map{"error": errTenantNotScoped})
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

		mismatches, err := deps.DB.Coordination().LedgerReconcileMismatches(c.UserContext(), orgID, since, limit)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{
			"window_hours": hours,
			"since":        since.Format(time.RFC3339),
			"rows":         mismatches,
			"count":        len(mismatches),
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
			return c.Status(403).JSON(fiber.Map{"error": errTenantNotScoped})
		}
		accountID, _ := strconv.ParseInt(c.Query("account_id", "0"), 10, 64)

		rows, err := deps.DB.Coordination().AccountHealthSnapshot(c.UserContext(), orgID, accountID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		// Normalize the trust_level fallback here so the dashboard doesn't
		// need to know the policy resolver's default. Empty string in the
		// row → "warming" on the wire.
		type wireRow struct {
			coordination.AccountHealthRow
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
