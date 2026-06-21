package presence

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/testsupport"
	"github.com/thg/scraper/internal/store"
)

func seedStatusAccount(t *testing.T, db *store.Store, owner int64, name, fbUser string) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: name,
		AssignedUserID: owner, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed account %s: %v", name, err)
	}
	// AddAccount does not persist fb_user_id; set the FB identity directly so the
	// derived state (identity match vs. wrong_account) is deterministic.
	if _, err := db.DB().Exec(`UPDATE accounts SET fb_user_id = ? WHERE id = ?`, fbUser, id); err != nil {
		t.Fatalf("set fb_user_id %s: %v", name, err)
	}
	return id
}

// seedStatusConnector inserts an agent_tokens row. lastSeenSQL is a raw SQL
// expression (CURRENT_TIMESTAMP for online; an old datetime for offline).
func seedStatusConnector(t *testing.T, db *store.Store, accID int64, fbUser, stream, lastSeenSQL string) {
	t.Helper()
	q := `INSERT INTO agent_tokens
		(org_id, name, created_by, token_hash, kind, transport, assigned_account_id,
		 fb_user_id, stream_status, version, active, last_seen, created_at)
	 VALUES (1, 'ext', 1, ?, 'extension_connector', 'chrome_extension', ?, ?, ?, '9.9.9', 1, ` + lastSeenSQL + `, CURRENT_TIMESTAMP)`
	if _, err := db.DB().Exec(q, fbUser, accID, fbUser, stream); err != nil {
		t.Fatalf("seed connector acc=%d: %v", accID, err)
	}
}

// TestConnectorStatus_StateMatrixAndPrivacy pins the presence board's derived
// states, the online/reachable counters, unbound-connector surfacing, and the
// PR-M5 account-privacy filter (a member sees only their own accounts).
func TestConnectorStatus_StateMatrixAndPrivacy(t *testing.T) {
	db := testsupport.NewTestStore(t, "connector_status.db")
	owner, _ := db.CreateUser(&models.User{OrgID: 1, Email: "owner@example.com", Name: "Owner", PasswordHash: "x", Role: models.RoleSales})
	other, _ := db.CreateUser(&models.User{OrgID: 1, Email: "other@example.com", Name: "Other", PasswordHash: "x", Role: models.RoleSales})

	online := seedStatusAccount(t, db, owner, "Online FB", "100")
	loggedOut := seedStatusAccount(t, db, owner, "LoggedOut FB", "200")
	wrong := seedStatusAccount(t, db, owner, "Wrong FB", "300")
	offline := seedStatusAccount(t, db, owner, "Offline FB", "400")
	seedStatusAccount(t, db, owner, "NoConnector FB", "500") // intentionally no connector
	seedStatusAccount(t, db, other, "Private FB", "600")     // owned by another member

	seedStatusConnector(t, db, online, "100", "facebook_logged_in", "CURRENT_TIMESTAMP")
	seedStatusConnector(t, db, loggedOut, "200", "idle", "CURRENT_TIMESTAMP")
	seedStatusConnector(t, db, wrong, "999", "facebook_logged_in", "CURRENT_TIMESTAMP") // connector logged into a different FB user
	seedStatusConnector(t, db, offline, "400", "facebook_logged_in", "datetime('now','-10 minutes')")
	seedStatusConnector(t, db, 0, "777", "facebook_logged_in", "CURRENT_TIMESTAMP") // unbound + online

	h := &Handler{db: db}
	app := fiber.New()
	app.Get("/connectors/status", func(c *fiber.Ctx) error {
		c.Locals("org_id", int64(1))
		c.Locals("user_id", owner)
		c.Locals("user_role", "sales")
		return h.connectorStatus(c)
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/connectors/status", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Accounts       []map[string]any `json:"accounts"`
		UnboundOnline  []map[string]any `json:"unbound_online"`
		AccountsTotal  int              `json:"accounts_total"`
		OnlineTotal    int              `json:"online_total"`
		ReachableTotal int              `json:"reachable_total"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v: %s", err, raw)
	}

	// Privacy: the other member's account is never visible — 5 own accounts shown.
	if out.AccountsTotal != 5 || len(out.Accounts) != 5 {
		t.Fatalf("accounts_total = %d (len %d), want 5 (privacy filtered)", out.AccountsTotal, len(out.Accounts))
	}
	// online_total counts accounts whose connector is online: online + logged_out + wrong = 3.
	if out.OnlineTotal != 3 {
		t.Errorf("online_total = %d, want 3", out.OnlineTotal)
	}
	// reachable_total counts only the fully-online state: 1.
	if out.ReachableTotal != 1 {
		t.Errorf("reachable_total = %d, want 1", out.ReachableTotal)
	}
	if len(out.UnboundOnline) != 1 || out.UnboundOnline[0]["state"] != "unassigned" {
		t.Errorf("unbound_online = %v, want exactly 1 unassigned", out.UnboundOnline)
	}

	stateByName := map[string]string{}
	for _, r := range out.Accounts {
		stateByName[r["account_name"].(string)] = r["state"].(string)
	}
	for name, st := range map[string]string{
		"Online FB":      "online",
		"LoggedOut FB":   "logged_out",
		"Wrong FB":       "wrong_account",
		"Offline FB":     "offline",
		"NoConnector FB": "no_connector",
	} {
		if stateByName[name] != st {
			t.Errorf("state[%s] = %q, want %q", name, stateByName[name], st)
		}
	}
	if _, leaked := stateByName["Private FB"]; leaked {
		t.Error("private account of another member leaked into the presence board")
	}
}
