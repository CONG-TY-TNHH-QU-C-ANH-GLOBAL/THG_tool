package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// seedAccountFB adds an account with a bound Facebook identity (fb_user_id is set by the
// heartbeat identity sync in prod; here we set it directly).
func seedAccountFB(t *testing.T, db *store.Store, org int64, name, fb string) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{OrgID: org, Platform: models.PlatformFacebook, Name: name, Status: models.AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE accounts SET fb_user_id = ? WHERE id = ?`, fb, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedLiveConnector inserts an online, logged-in extension connector logged into fb.
func seedLiveConnector(t *testing.T, db *store.Store, org, assignedAccount int64, fb string) {
	t.Helper()
	if _, err := db.DB().Exec(
		`INSERT INTO agent_tokens (org_id, name, created_by, token_hash, kind, transport,
			assigned_account_id, fb_user_id, stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', 1, ?, 'extension_connector', 'chrome_extension', ?, ?,
			'facebook_logged_in', '9.9.9', 1, datetime('now'), datetime('now'))`,
		org, "h"+fb, assignedAccount, fb); err != nil {
		t.Fatal(err)
	}
}

func liveConn(id, assignedAcct int64, fb string, online bool) connectors.AgentToken {
	return connectors.AgentToken{
		ID: id, AssignedAccountID: assignedAcct, FBUserID: fb,
		StreamStatus: "facebook_logged_in", Version: "9.9.9", Active: true, Online: online,
	}
}

// Pure decision matrix: liveReadyAccountIDs maps a live connector identity to its account,
// never first-ready, and reports ambiguity when more than one matched account is live.
func TestLiveReadyAccountIDs(t *testing.T) {
	pol := connectors.VersionPolicy{}
	fb2acc := func(m map[string]int64) func(string) int64 {
		return func(fb string) int64 { return m[fb] }
	}

	// (1) one live connector fb55 → account #55.
	got := liveReadyAccountIDs([]connectors.AgentToken{liveConn(1, 55, "fb55", true)}, pol, fb2acc(map[string]int64{"fb55": 55}))
	if len(got) != 1 || got[0] != 55 {
		t.Fatalf("single live identity must resolve to #55, got %v", got)
	}

	// (4) multiple ready accounts → both returned (caller treats as ambiguous, no first-ready).
	got = liveReadyAccountIDs(
		[]connectors.AgentToken{liveConn(1, 55, "fb55", true), liveConn(2, 77, "fb77", true)},
		pol, fb2acc(map[string]int64{"fb55": 55, "fb77": 77}))
	if len(got) != 2 {
		t.Fatalf("two live identities must both surface (ambiguous), got %v", got)
	}

	// no online connector → none.
	if got := liveReadyAccountIDs([]connectors.AgentToken{liveConn(1, 55, "fb55", false)}, pol, fb2acc(map[string]int64{"fb55": 55})); len(got) != 0 {
		t.Errorf("offline connector must yield no live account, got %v", got)
	}
	// live fb maps to NO account → excluded.
	if got := liveReadyAccountIDs([]connectors.AgentToken{liveConn(1, 0, "fbX", true)}, pol, fb2acc(map[string]int64{})); len(got) != 0 {
		t.Errorf("unmapped live fb must yield no account, got %v", got)
	}
	// logged-out connector (wrong stream) → excluded.
	loggedOut := liveConn(1, 55, "fb55", true)
	loggedOut.StreamStatus = "idle"
	if got := liveReadyAccountIDs([]connectors.AgentToken{loggedOut}, pol, fb2acc(map[string]int64{"fb55": 55})); len(got) != 0 {
		t.Errorf("logged-out connector must yield no live account, got %v", got)
	}
}

func TestConnectorBlockMessage(t *testing.T) {
	cases := map[string]string{
		connectors.ConnIdentityMismatch:        msgDPAccountMismatch,
		connectors.ConnIdentityUnknown:         msgDPAccountUnknown,
		connectors.ConnOffline:                 msgDPAccountOffline,
		connectors.ConnExtensionUpdateRequired: msgDPAccountVersion,
	}
	for reason, want := range cases {
		if got := connectorBlockMessage(reason); got != want {
			t.Errorf("reason %q → %q, want %q", reason, got, want)
		}
	}
}

// Integration: resolveDirectPostAccount over a real store with seeded accounts + a live
// connector. Covers selected-match (#A), selected-mismatch (block, never silent), and the
// no-selection unique-live resolve.
func TestResolveDirectPostAccount(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "acctguard.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const org int64 = 5
	accA, err := db.Identities().AddAccount(&models.Account{OrgID: org, Platform: models.PlatformFacebook, Name: "A", Status: models.AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	accB, err := db.Identities().AddAccount(&models.Account{OrgID: org, Platform: models.PlatformFacebook, Name: "B", Status: models.AccountActive})
	if err != nil {
		t.Fatal(err)
	}
	// Bind each account's Facebook identity (in prod this is set by the heartbeat identity sync).
	if _, err := db.DB().Exec(`UPDATE accounts SET fb_user_id = 'fbA' WHERE id = ?`, accA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE accounts SET fb_user_id = 'fbB' WHERE id = ?`, accB); err != nil {
		t.Fatal(err)
	}
	// One live (online, logged-in) UNASSIGNED connector currently logged into fbA.
	if _, err := db.DB().Exec(
		`INSERT INTO agent_tokens (org_id, name, created_by, token_hash, kind, transport,
			assigned_account_id, fb_user_id, stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', 1, 'h1', 'extension_connector', 'chrome_extension', 0, 'fbA',
			'facebook_logged_in', '9.9.9', 1, datetime('now'), datetime('now'))`, org); err != nil {
		t.Fatal(err)
	}

	// Selected #A (matches the live identity) → resolves to #A.
	if r := resolveDirectPostAccount(db, org, accA); !r.ok || r.accountID != accA {
		t.Errorf("selected #A must resolve to #A, got %+v", r)
	}
	// Selected #B (live Chrome is fbA, NOT fbB) → BLOCKED, never silently #B.
	if r := resolveDirectPostAccount(db, org, accB); r.ok {
		t.Errorf("selected #B with live identity fbA must be blocked, got %+v", r)
	}
	// No selection → unique live identity (#A) is resolved (NOT first-ready).
	if r := resolveDirectPostAccount(db, org, 0); !r.ok || r.accountID != accA {
		t.Errorf("no selection must resolve to the unique live account #A, got %+v", r)
	}
}

// No selection with MULTIPLE live-matched accounts → ambiguous → fail closed (never picks one).
func TestResolveDirectPostAccount_AmbiguousMultiLive(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "ambig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const org int64 = 5
	accA := seedAccountFB(t, db, org, "A", "fbA")
	accB := seedAccountFB(t, db, org, "B", "fbB")
	seedLiveConnector(t, db, org, accA, "fbA")
	seedLiveConnector(t, db, org, accB, "fbB")

	if r := resolveDirectPostAccount(db, org, 0); r.ok {
		t.Errorf("two live identities must fail closed (ambiguous), got %+v", r)
	}
	// Each explicit selection still resolves to its own live-matched account.
	if r := resolveDirectPostAccount(db, org, accA); !r.ok || r.accountID != accA {
		t.Errorf("selected #A must resolve to #A, got %+v", r)
	}
	if r := resolveDirectPostAccount(db, org, accB); !r.ok || r.accountID != accB {
		t.Errorf("selected #B must resolve to #B, got %+v", r)
	}
}

// Every Facebook WRITE action routed through the agent handler fails closed when no live
// connector identity can be resolved — no workflow/outbound/post job is created. The broad
// read action (scrape_group) is NOT account-guarded and proceeds.
func TestAgentActionHandler_WriteActionsFailClosed(t *testing.T) {
	db, js := newIntakeEnv(t)
	handler := makeAgentActionHandler(db, js, ai.NewMessageGenerator("", ""), nil)

	for _, action := range []string{"comment_all_leads", "auto_comment", "inbox_all_leads", "create_job_post", "post_to_profile"} {
		out, err := handler(action, map[string]any{"org_id": int64(5), "nl_prompt": "x", "content": "x"})
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !strings.Contains(out, "đăng nhập trong Chrome") {
			t.Errorf("write action %s must fail closed on the live-account guard, got %q", action, out)
		}
	}

	// Broad read/crawl is unchanged — it must NOT hit the account guard.
	out, err := handler("scrape_group", map[string]any{"org_id": int64(5), "url": "https://www.facebook.com/groups/123456789"})
	if err != nil {
		t.Fatalf("scrape_group: %v", err)
	}
	if strings.Contains(out, "đăng nhập trong Chrome") {
		t.Errorf("broad crawl must NOT be account-guarded, got %q", out)
	}
}
