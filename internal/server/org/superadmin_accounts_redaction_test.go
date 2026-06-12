package org

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

func newTestStore(t *testing.T, name string) *store.Store {
	t.Helper()
	dst := storetest.CopyTemplate(t, bootstrapStore, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open from template: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// GET /admin/accounts (founder-gated) previously serialized the raw
// models.Account slice from GetAllAccounts — which returns DECRYPTED
// cookies plus proxy_url/user_agent. The AccountSafe projection must
// strip every credential/infra field while keeping monitoring fields.
func TestSuperAdminAccounts_RedactsSecrets(t *testing.T) {
	db := newTestStore(t, "superadmin_redaction.db")
	if _, err := db.Identities().AddAccount(&models.Account{
		OrgID:          1,
		Platform:       models.PlatformFacebook,
		Name:           "Org1 FB",
		Email:          "staff@example.com",
		CookiesJSON:    `[{"name":"c_user","value":"SECRET_COOKIE_VALUE"}]`,
		ProxyURL:       "http://user:SECRET_PROXY_PASS@proxy.example.com:8080",
		UserAgent:      "Mozilla/5.0 SECRET_UA_FINGERPRINT",
		Status:         models.AccountActive,
		AssignedUserID: 9,
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	h := &Handler{deps: Deps{DB: db}}
	app := fiber.New()
	app.Get("/admin/accounts", h.superAdminAccounts)

	resp, err := app.Test(httptest.NewRequest("GET", "/admin/accounts", nil))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	for _, banned := range []string{
		"cookies_json", "CookiesJSON",
		"proxy_url", "ProxyURL",
		"user_agent", "UserAgent",
		"token", "session", "raw_session",
		"SECRET_COOKIE_VALUE", "SECRET_PROXY_PASS", "SECRET_UA_FINGERPRINT",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("superadmin accounts response contains forbidden %q:\n%s", banned, body)
		}
	}

	// Monitoring fields must survive the projection.
	for _, want := range []string{`"name":"Org1 FB"`, `"assigned_user_id":9`, `"status":"active"`} {
		if !strings.Contains(body, want) {
			t.Errorf("superadmin accounts response missing %s:\n%s", want, body)
		}
	}
}
