package main

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// seedOwnedAccountFB adds an active Facebook account assigned to ownerUserID with a bound
// Facebook identity (set directly here; in prod the heartbeat identity sync sets fb_user_id).
func seedOwnedAccountFB(t *testing.T, db *store.Store, org, ownerUserID int64, name, fb string) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{
		OrgID: org, Platform: models.PlatformFacebook, Name: name,
		Status: models.AccountActive, AssignedUserID: ownerUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE accounts SET fb_user_id = ? WHERE id = ?`, fb, id); err != nil {
		t.Fatal(err)
	}
	return id
}

// seedOwnedConnector inserts an online, logged-in extension connector OWNED by createdBy,
// assigned to assignedAccount and logged into fb.
func seedOwnedConnector(t *testing.T, db *store.Store, org, createdBy, assignedAccount int64, fb string) {
	t.Helper()
	if _, err := db.DB().Exec(
		`INSERT INTO agent_tokens (org_id, name, created_by, token_hash, kind, transport,
			assigned_account_id, fb_user_id, stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', ?, ?, 'extension_connector', 'chrome_extension', ?, ?,
			'facebook_logged_in', '9.9.9', 1, datetime('now'), datetime('now'))`,
		org, createdBy, "h"+fb, assignedAccount, fb); err != nil {
		t.Fatal(err)
	}
}

func newScopeStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Control predicate matrix (ACCOUNT side): role and visibility grant NOTHING. A member's
// account is controllable only by its owner; an unassigned account is allowed at the account
// level (real control still needs the requester's own live connector, checked at execution);
// an unproven requester (0) can control nothing.
func TestCanRequesterControlAccount(t *testing.T) {
	const userA, userB int64 = 11, 22
	own := &models.Account{ID: 1, AssignedUserID: userA}
	member := &models.Account{ID: 2, AssignedUserID: userB}
	unassigned := &models.Account{ID: 3, AssignedUserID: 0}

	if !canRequesterControlAccount(own, userA) {
		t.Error("owner must control their own account")
	}
	if canRequesterControlAccount(member, userA) {
		t.Error("a member must NOT control another member's account")
	}
	// Admin role is NOT passed and grants nothing — a member's account stays uncontrollable.
	if canRequesterControlAccount(member, 99 /* would-be admin id */) {
		t.Error("no requester controls a member's account by id alone (role grants nothing)")
	}
	// Unassigned org account is allowed at the account level (connector ownership re-checked).
	if !canRequesterControlAccount(unassigned, userA) {
		t.Error("unassigned org account must be allowed at the account level")
	}
	// Unproven requester (0) → controls nothing (fail closed).
	if canRequesterControlAccount(member, 0) || canRequesterControlAccount(own, 0) || canRequesterControlAccount(unassigned, 0) {
		t.Error("requester 0 must control nothing (identity required)")
	}
}

// PR-2 distributed pool: eligible_pool = live_matched ∩ requester_controllable. Other members'
// live accounts are excluded — never enlisted, never fatal.
func TestResolveControllablePool_OwnershipFilter(t *testing.T) {
	db := newScopeStore(t)
	const org, userA, userB int64 = 5, 11, 22

	a1 := seedOwnedAccountFB(t, db, org, userA, "A1", "fbA1")
	a2 := seedOwnedAccountFB(t, db, org, userA, "A2", "fbA2")
	b1 := seedOwnedAccountFB(t, db, org, userB, "B1", "fbB1")
	seedOwnedConnector(t, db, org, userA, a1, "fbA1")
	seedOwnedConnector(t, db, org, userA, a2, "fbA2")
	seedOwnedConnector(t, db, org, userB, b1, "fbB1")

	// User A: 2 own live accounts; User B: 1 live account → pool is exactly A's two.
	pool, blockMsg := resolveControllablePool(db, org, 0, userA)
	if blockMsg != "" {
		t.Fatalf("A has eligible accounts, must not block: %q", blockMsg)
	}
	if !sameIDSet(pool, []int64{a1, a2}) {
		t.Fatalf("pool must be exactly A's accounts %v, got %v (must exclude B's %d)", []int64{a1, a2}, pool, b1)
	}

	// User B's own account is never enlisted into A's pool.
	for _, id := range pool {
		if id == b1 {
			t.Fatalf("pool must NOT contain another member's account #%d", b1)
		}
	}

	// Explicit selection of an OWN account narrows the pool to that one.
	pool, blockMsg = resolveControllablePool(db, org, a1, userA)
	if blockMsg != "" || !sameIDSet(pool, []int64{a1}) {
		t.Fatalf("explicit own selection must yield [#%d], got %v / %q", a1, pool, blockMsg)
	}

	// Explicit selection of a MEMBER's account is blocked (never honoured).
	if _, blockMsg = resolveControllablePool(db, org, b1, userA); blockMsg != msgDPAccountNotControllable {
		t.Fatalf("selecting member account must block as not-controllable, got %q", blockMsg)
	}
}

// PR-2: when the requester has NO eligible account but another member has a live account,
// the pool fails closed — it must NEVER fall back to the member's account.
func TestResolveControllablePool_EmptyForRequester_NeverUsesMember(t *testing.T) {
	db := newScopeStore(t)
	const org, userA, userB int64 = 5, 11, 22
	b1 := seedOwnedAccountFB(t, db, org, userB, "B1", "fbB1")
	seedOwnedConnector(t, db, org, userB, b1, "fbB1")

	pool, blockMsg := resolveControllablePool(db, org, 0, userA)
	if blockMsg != msgDPNoControllableLive {
		t.Fatalf("A with no eligible account must fail closed, got pool=%v block=%q", pool, blockMsg)
	}
	if len(pool) != 0 {
		t.Fatalf("pool must be empty, never the member's account, got %v", pool)
	}
}

// Item 1: an unproven requester (0) must fail closed for a Facebook WRITE pool — never resolve
// org-wide, even when a live account exists in the org.
func TestResolveControllablePool_RequesterRequired(t *testing.T) {
	db := newScopeStore(t)
	const org, userB int64 = 5, 22
	b1 := seedOwnedAccountFB(t, db, org, userB, "B1", "fbB1")
	seedOwnedConnector(t, db, org, userB, b1, "fbB1")

	if pool, blockMsg := resolveControllablePool(db, org, 0, 0); blockMsg != msgDPRequesterRequired || len(pool) != 0 {
		t.Fatalf("requester 0 must fail closed (identity required), got pool=%v block=%q", pool, blockMsg)
	}
	// Even an explicit selection cannot proceed without a proven requester.
	if _, blockMsg := resolveControllablePool(db, org, b1, 0); blockMsg != msgDPRequesterRequired {
		t.Fatalf("requester 0 with explicit selection must fail closed, got %q", blockMsg)
	}
}

// sameIDSet reports whether got contains exactly the ids in want (order-independent).
func sameIDSet(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int64]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		seen[id]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
