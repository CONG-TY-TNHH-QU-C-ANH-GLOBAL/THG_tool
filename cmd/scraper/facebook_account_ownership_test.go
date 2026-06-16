package main

import "testing"

// Single-task ownership, member-isolation scenario: A has an account but NO live connector;
// only B's connector is live. Selecting / pre-picking / auto-resolving must never reach B's
// account, and admin does not bypass. (Seed helpers live in facebook_account_scope_test.go.)
func TestResolveDirectPostAccount_Ownership(t *testing.T) {
	db := newScopeStore(t)
	const org, userA, userB, admin int64 = 5, 11, 22, 99

	seedOwnedAccountFB(t, db, org, userA, "A1", "fbA1") // A's account, no live connector
	b1 := seedOwnedAccountFB(t, db, org, userB, "B1", "fbB1")
	seedOwnedConnector(t, db, org, userB, b1, "fbB1") // only B is live

	// User A explicitly selects User B's account → blocked.
	if r := resolveDirectPostAccount(db, org, b1, userA); r.ok {
		t.Errorf("A selecting B's account must block, got %+v", r)
	}
	// Brain/upstream pre-pick of a member's account_id is treated as a selection and blocked.
	if r := resolveDirectPostAccount(db, org, b1, userA); r.message != msgDPAccountNotControllable {
		t.Errorf("member pre-pick must block as not-controllable, got %q", r.message)
	}
	// Admin selecting a member's account → blocked (visibility != control).
	if r := resolveDirectPostAccount(db, org, b1, admin); r.ok {
		t.Errorf("admin must NOT control a member's account, got %+v", r)
	}
	// User A with no selection and only B's connector live → blocked, never resolves B.
	if r := resolveDirectPostAccount(db, org, 0, userA); r.ok {
		t.Errorf("A must not auto-resolve a member's live account, got %+v", r)
	}
}

// Single-task ownership, own-account scenario: A's own live account resolves on explicit
// selection and on the unique no-selection path.
func TestResolveDirectPostAccount_OwnAllowed(t *testing.T) {
	db := newScopeStore(t)
	const org, userA int64 = 5, 11
	a1 := seedOwnedAccountFB(t, db, org, userA, "A1", "fbA1")
	seedOwnedConnector(t, db, org, userA, a1, "fbA1")

	// User A selecting their OWN live account → allowed.
	if r := resolveDirectPostAccount(db, org, a1, userA); !r.ok || r.accountID != a1 {
		t.Errorf("A selecting own live account must resolve to #%d, got %+v", a1, r)
	}
	// User A with exactly one OWN live account and no selection → auto-use.
	if r := resolveDirectPostAccount(db, org, 0, userA); !r.ok || r.accountID != a1 {
		t.Errorf("A's unique own live account must auto-resolve to #%d, got %+v", a1, r)
	}
}

// Two OWN live accounts with no selection → ambiguous (ask to choose), never fan out, never
// first-ready.
func TestResolveDirectPostAccount_OwnAmbiguous(t *testing.T) {
	db := newScopeStore(t)
	const org, userA int64 = 5, 11
	a1 := seedOwnedAccountFB(t, db, org, userA, "A1", "fbA1")
	a2 := seedOwnedAccountFB(t, db, org, userA, "A2", "fbA2")
	seedOwnedConnector(t, db, org, userA, a1, "fbA1")
	seedOwnedConnector(t, db, org, userA, a2, "fbA2")
	if r := resolveDirectPostAccount(db, org, 0, userA); r.ok {
		t.Errorf("two own live accounts must be ambiguous (ask to choose), got %+v", r)
	}
}
