package main

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// resolveCallerAccountID gates skill-path outbound by execution-layer ownership
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
	aliceAccID, _ := db.AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Alice FB",
		AssignedUserID: 7, Status: models.AccountActive,
	})
	bobAccID, _ := db.AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Bob FB",
		AssignedUserID: 8, Status: models.AccountActive,
	})
	unassignedID, _ := db.AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Spare FB",
		AssignedUserID: 0, Status: models.AccountActive,
	})

	t.Run("sales explicit owned", func(t *testing.T) {
		got, err := resolveCallerAccountID(db, 1, 7, "sales", aliceAccID, false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != aliceAccID {
			t.Errorf("got %d, want %d", got, aliceAccID)
		}
	})

	t.Run("sales explicit NOT owned", func(t *testing.T) {
		_, err := resolveCallerAccountID(db, 1, 7, "sales", bobAccID, false)
		if err == nil {
			t.Fatal("expected error when sales targets another staff's account")
		}
	})

	t.Run("sales no account, has assignment", func(t *testing.T) {
		got, err := resolveCallerAccountID(db, 1, 7, "sales", 0, false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != aliceAccID {
			t.Errorf("got %d, want %d (alice's only assigned)", got, aliceAccID)
		}
	})

	t.Run("sales no account, no assignment", func(t *testing.T) {
		_, err := resolveCallerAccountID(db, 1, 99, "sales", 0, false)
		if err == nil {
			t.Fatal("expected error when sales has no assigned account")
		}
	})

	t.Run("admin explicit on another's account", func(t *testing.T) {
		// Admin user 5; should pass even though account_id is assigned to user 7.
		got, err := resolveCallerAccountID(db, 1, 5, "admin", aliceAccID, false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != aliceAccID {
			t.Errorf("got %d, want %d (admin override)", got, aliceAccID)
		}
	})

	t.Run("admin no account, picks any", func(t *testing.T) {
		got, err := resolveCallerAccountID(db, 1, 5, "admin", 0, false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		// admin sees the whole org pool; we just verify some account is picked.
		if got != aliceAccID && got != bobAccID && got != unassignedID {
			t.Errorf("got %d, want one of org accounts", got)
		}
	})

	t.Run("legacy unauthenticated picks any (telegram path)", func(t *testing.T) {
		got, err := resolveCallerAccountID(db, 1, 0, "", 0, false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got == 0 {
			t.Error("legacy path must resolve to some account")
		}
	})

	t.Run("sales targets unassigned account is rejected", func(t *testing.T) {
		_, err := resolveCallerAccountID(db, 1, 7, "sales", unassignedID, false)
		if err == nil {
			t.Fatal("sales must NOT be able to use an unassigned account")
		}
	})
}
