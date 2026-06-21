package org

import (
	"errors"
	"strconv"
	"strings"

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

func (h *Handler) superAdminUsers(c *fiber.Ctx) error {
	rows, err := h.deps.DB.DB().Query(
		`SELECT id, COALESCE(org_id,0), name, email, role, COALESCE(active,0), COALESCE(created_at,'')
		 FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type userRow struct {
		ID        int64  `json:"id"`
		OrgID     int64  `json:"org_id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		Active    int    `json:"active"`
		CreatedAt string `json:"created_at"`
	}
	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.OrgID, &u.Name, &u.Email, &u.Role, &u.Active, &u.CreatedAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		users = append(users, u)
	}
	return c.JSON(fiber.Map{"users": users, "count": len(users)})
}

func (h *Handler) superAdminSessions(c *fiber.Ctx) error {
	rows, err := h.deps.DB.DB().Query(
		`SELECT account_id, COALESCE(org_id,0), status,
		        COALESCE(cdp_port,0), COALESCE(vnc_port,0),
		        COALESCE(started_at,''), COALESCE(last_active_at,'')
		 FROM browser_sessions WHERE status != 'terminated'
		 ORDER BY started_at DESC`,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type sessionRow struct {
		AccountID    int64  `json:"account_id"`
		OrgID        int64  `json:"org_id"`
		Status       string `json:"status"`
		CDPPort      int64  `json:"cdp_port"`
		VNCPort      int64  `json:"vnc_port"`
		StartedAt    string `json:"started_at"`
		LastActiveAt string `json:"last_active_at"`
	}
	var sessions []sessionRow
	for rows.Next() {
		var ss sessionRow
		if err := rows.Scan(&ss.AccountID, &ss.OrgID, &ss.Status, &ss.CDPPort, &ss.VNCPort, &ss.StartedAt, &ss.LastActiveAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		sessions = append(sessions, ss)
	}
	return c.JSON(fiber.Map{"sessions": sessions, "count": len(sessions)})
}

func (h *Handler) superAdminDeleteOrg(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidID})
	}
	if id == 1 {
		return c.Status(403).JSON(fiber.Map{"error": "cannot delete platform org"})
	}
	if _, err := h.deps.DB.DB().ExecContext(c.Context(), `DELETE FROM organizations WHERE id = ?`, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
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

func (h *Handler) superAdminDeleteUser(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidID})
	}
	// Prevent self-delete
	if selfID, _ := c.Locals("user_id").(int64); selfID == id {
		return c.Status(403).JSON(fiber.Map{"error": "cannot delete your own account"})
	}
	if err := h.deps.DB.DeleteUser(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) superAdminTerminateSession(c *fiber.Ctx) error {
	accountID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": msgInvalidID})
	}
	if h.deps.Workspace != nil {
		h.deps.Workspace.Stop(accountID)
	}
	_, err = h.deps.DB.DB().ExecContext(c.Context(),
		`UPDATE browser_sessions SET status = 'terminated' WHERE account_id = ?`, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

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
	type attempt struct {
		StartedAt     string `json:"started_at"`
		ActionType    string `json:"action_type"`
		Outcome       string `json:"outcome"`
		FailureReason string `json:"failure_reason"`
		Notes         string `json:"notes"`
		PageURLAfter  string `json:"page_url_after"`
		DOMSnippet    string `json:"dom_snippet"`
		EvidenceRaw   string `json:"evidence_raw"`
	}
	attemptsRows, err := db.QueryContext(c.Context(),
		`SELECT COALESCE(started_at,''), COALESCE(action_type,''),
		        COALESCE(outcome,''), COALESCE(failure_reason,''),
		        COALESCE(evidence_json,'')
		   FROM execution_attempts WHERE account_id = ?
		   ORDER BY started_at DESC LIMIT 20`, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer attemptsRows.Close()
	var attempts []attempt
	for attemptsRows.Next() {
		var a attempt
		if err := attemptsRows.Scan(&a.StartedAt, &a.ActionType, &a.Outcome, &a.FailureReason, &a.EvidenceRaw); err != nil {
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

	// 4. Recent action_ledger entries (outbound history)
	type ledgerEntry struct {
		PerformedAt string `json:"performed_at"`
		ActionType  string `json:"action_type"`
		TargetURL   string `json:"target_url"`
		Outcome     string `json:"outcome"`
		Reason      string `json:"reason"`
	}
	ledgerRows, err := db.QueryContext(c.Context(),
		`SELECT COALESCE(performed_at,''), COALESCE(action_type,''),
		        COALESCE(target_url,''), COALESCE(outcome,''), COALESCE(reason,'')
		   FROM action_ledger WHERE account_id = ?
		   ORDER BY performed_at DESC LIMIT 20`, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer ledgerRows.Close()
	var ledger []ledgerEntry
	for ledgerRows.Next() {
		var le ledgerEntry
		if err := ledgerRows.Scan(&le.PerformedAt, &le.ActionType, &le.TargetURL, &le.Outcome, &le.Reason); err != nil {
			continue
		}
		ledger = append(ledger, le)
	}

	return c.JSON(fiber.Map{
		"account":   acc,
		"behaviour": b,
		"attempts":  attempts,
		"ledger":    ledger,
	})
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

// extractEvidenceField pulls one string field out of a proof JSON blob
// without importing encoding/json — handles the simple
// `"<field>":"<value>"` pattern that ClassifyExtensionReport emits.
// Returns empty when field absent.
func extractEvidenceField(blob, field string) string {
	needle := `"` + field + `":`
	i := strings.Index(blob, needle)
	if i < 0 {
		return ""
	}
	rest := blob[i+len(needle):]
	rest = strings.TrimLeft(rest, " ")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	for j := 0; j < len(rest); j++ {
		if rest[j] == '\\' && j+1 < len(rest) {
			j++
			continue
		}
		if rest[j] == '"' {
			return rest[:j]
		}
	}
	return ""
}

// superAdminQuery runs a fixed, allowlisted diagnostic report selected by the
// request "report" key. Arbitrary request-provided SQL is NOT executed: the
// legacy "sql" field is accepted only to reject it with a clear 400 so old
// callers fail loudly instead of silently. See superadmin_reports.go.
func (h *Handler) superAdminQuery(c *fiber.Ctx) error {
	var body struct {
		Report string `json:"report"`
		SQL    string `json:"sql,omitempty"` // accepted only to reject legacy raw-SQL usage
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if strings.TrimSpace(body.SQL) != "" {
		return c.Status(400).JSON(fiber.Map{"error": "raw sql is no longer supported; pass a report key"})
	}
	rows, err := h.querySuperadminReport(c.Context(), body.Report)
	if err != nil {
		if errors.Is(err, errUnknownReport) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid report"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	cols, result, err := scanSuperadminReportRows(rows)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"columns": cols, "rows": result, "count": len(result)})
}
