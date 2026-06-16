package main

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

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
