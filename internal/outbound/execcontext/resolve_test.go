package execcontext

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// requireResolvedAccount asserts ResolveCallerAccountID succeeds and resolves to want.
func requireResolvedAccount(t *testing.T, db *store.Store, orgID, userID int64, role string, accID, want int64) {
	t.Helper()
	got, err := ResolveCallerAccountID(db, orgID, userID, role, accID, false)
	if err != nil {
		t.Fatalf("ResolveCallerAccountID(user=%d, role=%q, acc=%d): unexpected err: %v", userID, role, accID, err)
	}
	if got != want {
		t.Errorf("ResolveCallerAccountID(user=%d, role=%q, acc=%d) = %d, want %d", userID, role, accID, got, want)
	}
}

// requireResolveRejected asserts ResolveCallerAccountID denies the call.
func requireResolveRejected(t *testing.T, db *store.Store, orgID, userID int64, role string, accID int64) {
	t.Helper()
	if _, err := ResolveCallerAccountID(db, orgID, userID, role, accID, false); err == nil {
		t.Fatalf("ResolveCallerAccountID(user=%d, role=%q, acc=%d): expected error, got nil", userID, role, accID)
	}
}

// requireResolvedOneOf asserts the call succeeds and resolves to one of allowed.
func requireResolvedOneOf(t *testing.T, db *store.Store, orgID, userID int64, role string, accID int64, allowed ...int64) {
	t.Helper()
	got, err := ResolveCallerAccountID(db, orgID, userID, role, accID, false)
	if err != nil {
		t.Fatalf("ResolveCallerAccountID(user=%d, role=%q, acc=%d): unexpected err: %v", userID, role, accID, err)
	}
	if !slices.Contains(allowed, got) {
		t.Errorf("ResolveCallerAccountID(user=%d, role=%q, acc=%d) = %d, want one of %v", userID, role, accID, got, allowed)
	}
}

// requireResolvedNonZero asserts the call succeeds and resolves to some account.
func requireResolvedNonZero(t *testing.T, db *store.Store, orgID, userID int64, role string, accID int64) {
	t.Helper()
	got, err := ResolveCallerAccountID(db, orgID, userID, role, accID, false)
	if err != nil {
		t.Fatalf("ResolveCallerAccountID(user=%d, role=%q, acc=%d): unexpected err: %v", userID, role, accID, err)
	}
	if got == 0 {
		t.Errorf("ResolveCallerAccountID(user=%d, role=%q, acc=%d) = 0, want some account", userID, role, accID)
	}
}

// ResolveCallerAccountID gates skill-path outbound by execution-layer ownership
// (RBAC-1 skill-path enforcement). See feedback_shared_battlefield_not_crm.md.
//
// Matrix per call:
//   - Sales staff with explicit account_id owned        → resolved.
//   - Sales staff with explicit account_id NOT owned    → error.
//   - Sales staff with no account_id, owns 1 account    → resolves to it.
//   - Sales staff with no account_id, owns no account   → error.
//   - Admin with explicit account_id (any in org)       → resolved.
//   - Admin with no account_id                          → picks first org account.
//   - Telegram bot (userID=0, role="") → preserves legacy "any account" path.
func TestResolveCallerAccountID(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "owner_test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	// Seed 3 accounts:
	//   id=A (Alice, sales, user 7)
	//   id=B (Bob,   sales, user 8)
	//   id=C (unassigned)
	aliceAccID, _ := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Alice FB",
		AssignedUserID: 7, Status: models.AccountActive,
	})
	bobAccID, _ := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Bob FB",
		AssignedUserID: 8, Status: models.AccountActive,
	})
	unassignedID, _ := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Spare FB",
		AssignedUserID: 0, Status: models.AccountActive,
	})

	t.Run("sales explicit owned", func(t *testing.T) {
		requireResolvedAccount(t, db, 1, 7, "sales", aliceAccID, aliceAccID)
	})

	t.Run("sales explicit NOT owned", func(t *testing.T) {
		requireResolveRejected(t, db, 1, 7, "sales", bobAccID)
	})

	t.Run("sales no account, has assignment", func(t *testing.T) {
		// Alice's only assigned account is resolved.
		requireResolvedAccount(t, db, 1, 7, "sales", 0, aliceAccID)
	})

	t.Run("sales no account, no assignment", func(t *testing.T) {
		requireResolveRejected(t, db, 1, 99, "sales", 0)
	})

	t.Run("admin explicit on another's account", func(t *testing.T) {
		// Admin user 5; should pass even though account_id is assigned to user 7.
		requireResolvedAccount(t, db, 1, 5, "admin", aliceAccID, aliceAccID)
	})

	t.Run("admin no account, picks any", func(t *testing.T) {
		// admin sees the whole org pool; we just verify some account is picked.
		requireResolvedOneOf(t, db, 1, 5, "admin", 0, aliceAccID, bobAccID, unassignedID)
	})

	t.Run("legacy unauthenticated picks any (telegram path)", func(t *testing.T) {
		requireResolvedNonZero(t, db, 1, 0, "", 0)
	})

	t.Run("sales targets unassigned account is rejected", func(t *testing.T) {
		requireResolveRejected(t, db, 1, 7, "sales", unassignedID)
	})
}
