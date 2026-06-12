package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// forbiddenAccountJSON is the closed list of substrings that must never
// appear in any HTTP serialization of an account. Keys cover both JSON
// tags and Go field names (a regression that re-adds the field under
// either casing fails); values cover the seeded secrets themselves.
var forbiddenAccountJSON = []string{
	"cookies_json", "CookiesJSON",
	"proxy_url", "ProxyURL",
	"user_agent", "UserAgent",
	"token", "session", "raw_session",
}

// fullyLoadedAccount returns an Account with every sensitive field
// populated the way the store layer returns it to handlers (cookies
// already DECRYPTED).
func fullyLoadedAccount() Account {
	return Account{
		ID:               42,
		OrgID:            7,
		Platform:         PlatformFacebook,
		Name:             "Alice FB",
		Email:            "alice@example.com",
		CookiesJSON:      `[{"name":"c_user","value":"SECRET_COOKIE_VALUE"}]`,
		ProxyURL:         "http://user:SECRET_PROXY_PASS@proxy.example.com:8080",
		UserAgent:        "Mozilla/5.0 SECRET_UA_FINGERPRINT",
		Status:           AccountActive,
		Notes:            "operator note",
		LastUsed:         time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		CreatedAt:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		AssignedUserID:   9,
		AssignedUserName: "Alice",
		BrowserLoggedIn:  true,
		FBUserID:         "100014197607233",
		FBDisplayName:    "Alice Nguyen",
		FBUsername:       "alice.nguyen",
		FBProfileURL:     "https://www.facebook.com/alice.nguyen",
	}
}

// AccountSafe is the ONLY shape handlers may serialize. Its JSON must
// carry the monitoring fields and none of the credential/infra fields.
func TestAccountSafe_RedactsSecrets(t *testing.T) {
	acc := fullyLoadedAccount()
	raw, err := json.Marshal(NewAccountSafe(&acc))
	if err != nil {
		t.Fatalf("marshal AccountSafe: %v", err)
	}
	body := string(raw)

	for _, banned := range forbiddenAccountJSON {
		if strings.Contains(body, banned) {
			t.Errorf("AccountSafe JSON contains forbidden substring %q:\n%s", banned, body)
		}
	}
	for _, secret := range []string{"SECRET_COOKIE_VALUE", "SECRET_PROXY_PASS", "SECRET_UA_FINGERPRINT"} {
		if strings.Contains(body, secret) {
			t.Errorf("AccountSafe JSON leaks secret value %q:\n%s", secret, body)
		}
	}

	// Safe fields the UI depends on (accountsService.ts mapAccount) must survive.
	for _, want := range []string{
		`"id":42`, `"org_id":7`, `"name":"Alice FB"`, `"status":"active"`,
		`"assigned_user_id":9`, `"assigned_user_name":"Alice"`,
		`"browser_logged_in":true`, `"fb_user_id":"100014197607233"`,
		`"fb_display_name":"Alice Nguyen"`, `"fb_profile_url":"https://www.facebook.com/alice.nguyen"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("AccountSafe JSON missing expected field %s:\n%s", want, body)
		}
	}
}

// Guard the raw model too: if Account ever drops the sensitive fields'
// JSON tags this test documents the contrast — the raw model DOES
// serialize secrets, which is exactly why it must never cross HTTP.
func TestAccountRawModel_IsNotSafeForHTTP(t *testing.T) {
	acc := fullyLoadedAccount()
	raw, err := json.Marshal(acc)
	if err != nil {
		t.Fatalf("marshal Account: %v", err)
	}
	if !strings.Contains(string(raw), "SECRET_COOKIE_VALUE") {
		t.Skip("Account no longer serializes cookies — revisit whether AccountSafe is still required")
	}
}

func TestAccountSafeList_EmptyIsJSONArray(t *testing.T) {
	raw, err := json.Marshal(AccountSafeList(nil))
	if err != nil {
		t.Fatalf("marshal empty AccountSafeList: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("empty AccountSafeList must render as [], got %s", raw)
	}
}
