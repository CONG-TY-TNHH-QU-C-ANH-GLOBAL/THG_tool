package presence

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/testsupport"
)

// fetchOverview issues the admin overview request (as admin user 99 in org 1)
// and returns the raw body plus the decoded account rows.
func fetchOverview(t *testing.T, app *fiber.App) (string, []map[string]any) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/admin/connectors/overview", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v: %s", err, raw)
	}
	return string(raw), out.Accounts
}

// assertNoSecretLeak fails if the overview body carries any credential/infra
// field even though the source account row holds them decrypted.
func assertNoSecretLeak(t *testing.T, body string) {
	t.Helper()
	for _, banned := range []string{"cookies_json", "proxy_url", "user_agent", "SECRET_COOKIE_VALUE", "SECRET_PROXY_PASS", "SECRET_UA", "token_hash"} {
		if strings.Contains(body, banned) {
			t.Errorf("overview leaks %q:\n%s", banned, body)
		}
	}
}

// assertOperationalRow pins the operational projection for the seeded account:
// staff binding, version state, pause/eligibility, block reasons.
func assertOperationalRow(t *testing.T, row map[string]any) {
	t.Helper()
	if row["staff_name"] != "Staff S" || row["staff_email"] != "staff@example.com" {
		t.Errorf("staff binding missing: %v", row)
	}
	// 0.5.10 is below the default supported floor → unsupported, blocked.
	if row["extension_version_state"] != "unsupported" {
		t.Errorf("version state = %v, want unsupported", row["extension_version_state"])
	}
	if row["assignment_paused"] != true || row["automation_eligible"] != false {
		t.Errorf("pause/eligibility wrong: %v", row)
	}
	joined := ""
	for _, r := range row["block_reasons"].([]any) {
		joined += r.(string) + ","
	}
	if !strings.Contains(joined, "extension_unsupported") || !strings.Contains(joined, "assignment_paused_by_admin") {
		t.Errorf("block reasons missing: %v", row["block_reasons"])
	}
}

// PR-3 admin overview: operational fields only (staff, FB display name,
// connector, version state, eligibility, pause) — and NEVER cookies /
// proxy / user-agent / session data, even though the source account row
// carries them decrypted.
func TestConnectorOverview_OperationalOnly(t *testing.T) {
	db := testsupport.NewTestStore(t, "connector_overview.db")

	staffID, _ := db.CreateUser(&models.User{OrgID: 1, Email: "staff@example.com", Name: "Staff S", PasswordHash: "x", Role: models.RoleSales})
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Staff FB",
		AssignedUserID: staffID, Status: models.AccountActive,
		CookiesJSON: `[{"name":"c_user","value":"SECRET_COOKIE_VALUE"}]`,
		ProxyURL:    "http://user:SECRET_PROXY_PASS@proxy:8080",
		UserAgent:   "SECRET_UA",
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.DB().Exec(
		`INSERT INTO agent_tokens
			(org_id, name, created_by, token_hash, kind, transport, assigned_account_id,
			 fb_user_id, stream_status, version, active, last_seen, created_at)
		 VALUES (1, 'ext', ?, 'h1', 'extension_connector', 'chrome_extension', ?,
		        '111', 'facebook_logged_in', '0.5.10', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		staffID, accID,
	); err != nil {
		t.Fatalf("seed connector: %v", err)
	}
	_ = db.Identities().SetAccountAssignmentPaused(accID, 1, true)

	h := &Handler{db: db}
	app := fiber.New()
	app.Get("/admin/connectors/overview", func(c *fiber.Ctx) error {
		c.Locals("org_id", int64(1))
		c.Locals("user_id", int64(99))
		c.Locals("user_role", "admin")
		return h.connectorOverview(c)
	})

	body, rows := fetchOverview(t, app)
	assertNoSecretLeak(t, body)
	if len(rows) != 1 {
		t.Fatalf("want 1 account row, got %d: %s", len(rows), body)
	}
	assertOperationalRow(t, rows[0])
	// Contact-profile audit column (review item 3): no profile → "missing".
	if rows[0]["contact_profile_state"] != "missing" {
		t.Errorf("contact_profile_state = %v, want missing", rows[0]["contact_profile_state"])
	}

	// An active profile with a usable line → "complete".
	if err := db.UpsertStaffContactProfile(&models.StaffContactProfile{
		UserID: staffID, OrgID: 1, Zalo: "0901234567", Active: true, Visibility: "team",
	}); err != nil {
		t.Fatalf("seed contact profile: %v", err)
	}
	_, rows2 := fetchOverview(t, app)
	if rows2[0]["contact_profile_state"] != "complete" {
		t.Errorf("contact_profile_state after seed = %v, want complete", rows2[0]["contact_profile_state"])
	}
}
