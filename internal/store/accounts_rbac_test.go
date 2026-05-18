package store

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// GetAccountsForUser must filter to a single staff's assigned accounts.
// Backs the sales-staff account list view per RBAC-1.
func TestGetAccountsForUser_FiltersToAssigned(t *testing.T) {
	db := newSharedStore(t, "rbac_accounts.db")

	// Seed 3 accounts: 2 assigned to user 7 (Alice), 1 assigned to user 8 (Bob).
	seed := []models.Account{
		{OrgID: 1, Platform: models.PlatformFacebook, Name: "Alice FB 1", AssignedUserID: 7, Status: models.AccountActive},
		{OrgID: 1, Platform: models.PlatformFacebook, Name: "Alice FB 2", AssignedUserID: 7, Status: models.AccountActive},
		{OrgID: 1, Platform: models.PlatformFacebook, Name: "Bob FB",     AssignedUserID: 8, Status: models.AccountActive},
	}
	for i := range seed {
		if _, err := db.AddAccount(&seed[i]); err != nil {
			t.Fatalf("AddAccount[%d]: %v", i, err)
		}
	}

	alice, err := db.GetAccountsForUser(1, 7)
	if err != nil {
		t.Fatalf("GetAccountsForUser(alice): %v", err)
	}
	if len(alice) != 2 {
		t.Fatalf("alice sees %d accounts, want 2", len(alice))
	}
	for _, a := range alice {
		if a.AssignedUserID != 7 {
			t.Errorf("alice got account assigned to user %d, want 7", a.AssignedUserID)
		}
	}

	bob, err := db.GetAccountsForUser(1, 8)
	if err != nil {
		t.Fatalf("GetAccountsForUser(bob): %v", err)
	}
	if len(bob) != 1 {
		t.Fatalf("bob sees %d accounts, want 1", len(bob))
	}
	if bob[0].Name != "Bob FB" {
		t.Errorf("bob got account %q, want \"Bob FB\"", bob[0].Name)
	}

	// Charlie (user 9) has nothing assigned — empty result.
	charlie, _ := db.GetAccountsForUser(1, 9)
	if len(charlie) != 0 {
		t.Errorf("charlie sees %d accounts, want 0", len(charlie))
	}

	// Admin uses GetAllAccounts — still sees all 3.
	all, err := db.GetAllAccounts(1)
	if err != nil {
		t.Fatalf("GetAllAccounts: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("admin sees %d accounts, want 3", len(all))
	}
}

func TestGetAccountsForUser_InvalidInputs(t *testing.T) {
	db := newSharedStore(t, "rbac_invalid.db")

	if accs, _ := db.GetAccountsForUser(0, 7); accs != nil {
		t.Error("org_id=0 must return nil (no leak to other orgs)")
	}
	if accs, _ := db.GetAccountsForUser(1, 0); accs != nil {
		t.Error("user_id=0 must return nil (avoid matching legacy unassigned rows)")
	}
}
