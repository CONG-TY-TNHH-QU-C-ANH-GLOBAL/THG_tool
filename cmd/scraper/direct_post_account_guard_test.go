package main

import (
	"testing"

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

// Integration: resolveDirectPostAccount over a real store with a requester-owned account + a
// requester-owned live connector. Covers selected-match (#A), selected-mismatch (block, never
// silent), and the no-selection unique-live resolve. (Seed helpers live in
// facebook_account_scope_test.go.)
func TestResolveDirectPostAccount(t *testing.T) {
	db := newScopeStore(t)
	const org, owner int64 = 5, 7
	accA := seedOwnedAccountFB(t, db, org, owner, "A", "fbA")
	accB := seedOwnedAccountFB(t, db, org, owner, "B", "fbB")
	seedOwnedConnector(t, db, org, owner, accA, "fbA") // owner's live connector → fbA

	// Selected #A (matches the live identity) → resolves to #A.
	if r := resolveDirectPostAccount(db, org, accA, owner); !r.ok || r.accountID != accA {
		t.Errorf("selected #A must resolve to #A, got %+v", r)
	}
	// Selected #B (live Chrome is fbA, NOT fbB) → BLOCKED, never silently #B.
	if r := resolveDirectPostAccount(db, org, accB, owner); r.ok {
		t.Errorf("selected #B with live identity fbA must be blocked, got %+v", r)
	}
	// No selection → unique live identity (#A) is resolved (NOT first-ready).
	if r := resolveDirectPostAccount(db, org, 0, owner); !r.ok || r.accountID != accA {
		t.Errorf("no selection must resolve to the unique live account #A, got %+v", r)
	}
}

// No selection with MULTIPLE own live-matched accounts → ambiguous → fail closed (never picks).
func TestResolveDirectPostAccount_AmbiguousMultiLive(t *testing.T) {
	db := newScopeStore(t)
	const org, owner int64 = 5, 7
	accA := seedOwnedAccountFB(t, db, org, owner, "A", "fbA")
	accB := seedOwnedAccountFB(t, db, org, owner, "B", "fbB")
	seedOwnedConnector(t, db, org, owner, accA, "fbA")
	seedOwnedConnector(t, db, org, owner, accB, "fbB")

	if r := resolveDirectPostAccount(db, org, 0, owner); r.ok {
		t.Errorf("two live identities must fail closed (ambiguous), got %+v", r)
	}
	// Each explicit selection still resolves to its own live-matched account.
	if r := resolveDirectPostAccount(db, org, accA, owner); !r.ok || r.accountID != accA {
		t.Errorf("selected #A must resolve to #A, got %+v", r)
	}
	if r := resolveDirectPostAccount(db, org, accB, owner); !r.ok || r.accountID != accB {
		t.Errorf("selected #B must resolve to #B, got %+v", r)
	}
}

