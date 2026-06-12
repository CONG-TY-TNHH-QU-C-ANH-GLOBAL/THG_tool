package workspace

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// getAccountsBody seeds two accounts (one owned by sales user 9, one by
// user 8), then calls GET /accounts as the given principal and returns
// the response body. Mirrors the production middleware by injecting the
// auth Locals the handler reads.
func getAccountsBody(t *testing.T, dbName string, userID int64, role string) string {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, dbName)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	seed := []models.Account{
		{OrgID: 1, Platform: models.PlatformFacebook, Name: "Mine FB", AssignedUserID: 9,
			CookiesJSON: `[{"name":"c_user","value":"SECRET_COOKIE_VALUE"}]`,
			ProxyURL:    "http://user:SECRET_PROXY_PASS@proxy.example.com:8080",
			UserAgent:   "Mozilla/5.0 SECRET_UA_FINGERPRINT",
			Status:      models.AccountActive},
		{OrgID: 1, Platform: models.PlatformFacebook, Name: "Colleague FB", AssignedUserID: 8,
			CookiesJSON: `[{"name":"c_user","value":"SECRET_COOKIE_VALUE"}]`,
			Status:      models.AccountActive},
	}
	for i := range seed {
		if _, err := db.Identities().AddAccount(&seed[i]); err != nil {
			t.Fatalf("seed account %d: %v", i, err)
		}
	}

	h := &Handler{db: db}
	app := fiber.New()
	app.Get("/accounts", func(c *fiber.Ctx) error {
		c.Locals("org_id", int64(1))
		c.Locals("user_id", userID)
		c.Locals("user_role", role)
		return h.getAccounts(c)
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/accounts", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	return string(raw)
}

func assertNoAccountSecrets(t *testing.T, body string) {
	t.Helper()
	for _, banned := range []string{
		"cookies_json", "CookiesJSON",
		"proxy_url", "ProxyURL",
		"user_agent", "UserAgent",
		"token", "session", "raw_session",
		"SECRET_COOKIE_VALUE", "SECRET_PROXY_PASS", "SECRET_UA_FINGERPRINT",
		"[REDACTED]", // the old cookie mask must be gone, not renamed
	} {
		if strings.Contains(body, banned) {
			t.Errorf("accounts response contains forbidden %q:\n%s", banned, body)
		}
	}
}

// Admin view: PR-M5 device privacy means admin sees own + unassigned
// accounts only — and whatever is visible must be the safe projection.
func TestGetAccounts_AdminResponseRedacted(t *testing.T) {
	body := getAccountsBody(t, "get_accounts_admin.db", 9, "admin")
	assertNoAccountSecrets(t, body)
	if !strings.Contains(body, `"name":"Mine FB"`) {
		t.Errorf("admin should see their own account:\n%s", body)
	}
}

// Sales view: only the caller's own account, fully redacted. The
// colleague's account must not appear at all (cross-member privacy).
func TestGetAccounts_SalesResponseRedactedAndOwnedOnly(t *testing.T) {
	body := getAccountsBody(t, "get_accounts_sales.db", 9, "sales")
	assertNoAccountSecrets(t, body)
	if !strings.Contains(body, `"name":"Mine FB"`) {
		t.Errorf("sales should see their own account:\n%s", body)
	}
	if strings.Contains(body, "Colleague FB") {
		t.Errorf("sales must not see another member's account:\n%s", body)
	}
}
