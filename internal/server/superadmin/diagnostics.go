package superadmin

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
)

// superAdminAccountDiagnostic returns aggregated diagnostic data for ONE
// account: behaviour profile + runtime state + recent execution_attempts
// with parsed evidence_json notes. Read-only, founder-gated.
//
// Built to unblock the redirected_feed diagnostic loop without forcing
// operators to write raw SQL — when an account starts failing in
// production, founder hits this endpoint, reads the landed_at_* notes
// from recent gate failures, and diagnoses without SuperAdmin SQL tab.
//
// Per project_runtime_control_plane memory: this is NOT a CRUD tab. It
// is a single-purpose diagnostic surface for a specific operational
// question ("why is account #X failing?"). The proper Runtime Control
// Plane (Runtime Feed + Execution Replay + Account Fleet + Routing
// Graph) is the EXP track; this endpoint is a precursor that survives
// because the diagnostic question outlives the temporary scaffold.
func (h *Handler) superAdminAccountDiagnostic(c *fiber.Ctx) error {
	accountID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || accountID <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid account id"})
	}

	db := h.deps.DB.DB()

	// 1. Account header
	type accountHeader struct {
		ID              int64  `json:"id"`
		OrgID           int64  `json:"org_id"`
		Name            string `json:"name"`
		Email           string `json:"email"`
		Status          string `json:"status"`
		BrowserLoggedIn bool   `json:"browser_logged_in"`
	}
	var acc accountHeader
	{
		var loggedIn int
		err = db.QueryRowContext(c.Context(),
			`SELECT id, COALESCE(org_id,0), COALESCE(name,''), COALESCE(email,''),
			        COALESCE(status,''), COALESCE(browser_logged_in,0)
			   FROM accounts WHERE id = ?`, accountID,
		).Scan(&acc.ID, &acc.OrgID, &acc.Name, &acc.Email, &acc.Status, &loggedIn)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "account not found"})
		}
		acc.BrowserLoggedIn = loggedIn != 0
	}

	// 2. Behaviour profile + runtime state (cross-org allowed for founder)
	type behaviour struct {
		TrustLevel      string  `json:"trust_level"`
		RiskScore       float64 `json:"risk_score"`
		Ceiling         float64 `json:"ceiling"`
		RecentFailures  int     `json:"recent_failures"`
		CooldownUntil   string  `json:"cooldown_until"`
		CommentsToday   int     `json:"comments_today"`
		InboxToday      int     `json:"inbox_today"`
		GroupPostsToday int     `json:"group_posts_today"`
		CountersDay     string  `json:"counters_day"`
		RiskCeilingHit  bool    `json:"risk_ceiling_hit"`
	}
	var b behaviour
	// Profile (may not exist for fresh accounts — TrustWarming default applies)
	_ = db.QueryRowContext(c.Context(),
		`SELECT COALESCE(trust_level,''), COALESCE(risk_ceiling,0)
		   FROM account_behaviour_profiles WHERE account_id = ?`, accountID,
	).Scan(&b.TrustLevel, &b.Ceiling)
	// Runtime state (may not exist for fresh accounts)
	_ = db.QueryRowContext(c.Context(),
		`SELECT COALESCE(risk_score,0), COALESCE(recent_failures,0),
		        COALESCE(cooldown_until,''), COALESCE(comments_today,0),
		        COALESCE(inbox_today,0), COALESCE(group_posts_today,0),
		        COALESCE(counters_day,'')
		   FROM account_runtime_state WHERE account_id = ?`, accountID,
	).Scan(&b.RiskScore, &b.RecentFailures, &b.CooldownUntil,
		&b.CommentsToday, &b.InboxToday, &b.GroupPostsToday, &b.CountersDay)
	// Compute the EFFECTIVE ceiling the runtime gate actually uses.
	// The profile-level risk_ceiling override is rarely set (legacy field);
	// the real ceiling comes from the resolved trust preset (TrustWarming
	// → 0.60 by default). Reporting only the profile field caused this
	// surface to show risk_ceiling_hit=false while the runtime was
	// rejecting every queue with risk_ceiling_exceeded.
	trust := models.NormalizeTrustLevel(b.TrustLevel)
	resolvedCaps := models.ResolveBehaviourCaps(trust, "")
	effectiveCeiling := b.Ceiling
	if effectiveCeiling <= 0 {
		effectiveCeiling = resolvedCaps.RiskScoreCeiling
	}
	b.Ceiling = effectiveCeiling
	b.RiskCeilingHit = effectiveCeiling > 0 && b.RiskScore >= effectiveCeiling
	if b.TrustLevel == "" {
		b.TrustLevel = string(trust) + " (default)"
	}

	// 3. Recent execution_attempts with parsed notes (THE diagnostic gold)
	attempts, err := h.loadRecentAttempts(c.Context(), accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	// 4. Recent action_ledger entries (outbound history)
	ledger, err := h.loadRecentLedger(c.Context(), accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"account":   acc,
		"behaviour": b,
		"attempts":  attempts,
		"ledger":    ledger,
	})
}

type accountAttempt struct {
	StartedAt     string `json:"started_at"`
	ActionType    string `json:"action_type"`
	Outcome       string `json:"outcome"`
	FailureReason string `json:"failure_reason"`
	Notes         string `json:"notes"`
	PageURLAfter  string `json:"page_url_after"`
	DOMSnippet    string `json:"dom_snippet"`
	EvidenceRaw   string `json:"evidence_raw"`
}

// loadRecentAttempts returns the most recent execution_attempts for an account,
// with proof fields parsed out of each evidence_json blob.
func (h *Handler) loadRecentAttempts(ctx context.Context, accountID int64) ([]accountAttempt, error) {
	rows, err := h.deps.DB.DB().QueryContext(ctx,
		`SELECT COALESCE(started_at,''), COALESCE(action_type,''),
		        COALESCE(outcome,''), COALESCE(failure_reason,''),
		        COALESCE(evidence_json,'')
		   FROM execution_attempts WHERE account_id = ?
		   ORDER BY started_at DESC LIMIT 20`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attempts []accountAttempt
	for rows.Next() {
		var a accountAttempt
		if err := rows.Scan(&a.StartedAt, &a.ActionType, &a.Outcome, &a.FailureReason, &a.EvidenceRaw); err != nil {
			continue
		}
		a.Notes = extractEvidenceField(a.EvidenceRaw, "notes")
		a.PageURLAfter = extractEvidenceField(a.EvidenceRaw, "page_url_after")
		a.DOMSnippet = extractEvidenceField(a.EvidenceRaw, "dom_snippet")
		if len(a.DOMSnippet) > 300 {
			a.DOMSnippet = a.DOMSnippet[:300] + "…"
		}
		attempts = append(attempts, a)
	}
	return attempts, nil
}

type accountLedgerEntry struct {
	PerformedAt string `json:"performed_at"`
	ActionType  string `json:"action_type"`
	TargetURL   string `json:"target_url"`
	Outcome     string `json:"outcome"`
	Reason      string `json:"reason"`
}

// loadRecentLedger returns the most recent action_ledger entries for an account.
func (h *Handler) loadRecentLedger(ctx context.Context, accountID int64) ([]accountLedgerEntry, error) {
	rows, err := h.deps.DB.DB().QueryContext(ctx,
		`SELECT COALESCE(performed_at,''), COALESCE(action_type,''),
		        COALESCE(target_url,''), COALESCE(outcome,''), COALESCE(reason,'')
		   FROM action_ledger WHERE account_id = ?
		   ORDER BY performed_at DESC LIMIT 20`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ledger []accountLedgerEntry
	for rows.Next() {
		var le accountLedgerEntry
		if err := rows.Scan(&le.PerformedAt, &le.ActionType, &le.TargetURL, &le.Outcome, &le.Reason); err != nil {
			continue
		}
		ledger = append(ledger, le)
	}
	return ledger, nil
}
