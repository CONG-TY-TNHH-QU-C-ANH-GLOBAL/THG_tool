package copilot

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

func newAccountContextStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "acct-context.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedCtxAccount(t *testing.T, db *store.Store, org, owner int64, name string) int64 {
	t.Helper()
	id, err := db.Identities().AddAccount(&models.Account{
		OrgID: org, Platform: models.PlatformFacebook, Name: name,
		Status: models.AccountActive, AssignedUserID: owner,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func hasAccount(accs []models.Account, id int64) bool {
	for _, a := range accs {
		if a.ID == id {
			return true
		}
	}
	return false
}

// PR-1 LLM context isolation: the action-planning account context must contain ONLY the
// requester-controllable accounts — never another member's account, even for an admin.
func TestAccountsForActionPlanning_Isolation(t *testing.T) {
	db := newAccountContextStore(t)
	const org, userA, userB, admin int64 = 7, 11, 22, 99
	a1 := seedCtxAccount(t, db, org, userA, "A1")
	b1 := seedCtxAccount(t, db, org, userB, "B1")
	unassigned := seedCtxAccount(t, db, org, 0, "OrgOwned")

	agent := NewAgent("k", "m", db)

	// User A (sales) sees only their own account — not B's. The unassigned org account is
	// plannable (control re-checked at execution), but never another member's account.
	a := agent.accountsForActionPlanning(org, userA)
	if !hasAccount(a, a1) || hasAccount(a, b1) {
		t.Fatalf("A's planning context must include A1 and exclude B1, got %v", accountIDs(a))
	}
	if !hasAccount(a, unassigned) {
		t.Fatalf("A's planning context should include the unassigned org account, got %v", accountIDs(a))
	}

	// Admin sees own (none here) + unassigned, but NEVER a member's account (role grants nothing).
	adm := agent.accountsForActionPlanning(org, admin)
	if hasAccount(adm, a1) || hasAccount(adm, b1) {
		t.Fatalf("admin planning context must NOT include member accounts, got %v", accountIDs(adm))
	}
	if !hasAccount(adm, unassigned) {
		t.Fatalf("admin planning context should include the unassigned org account, got %v", accountIDs(adm))
	}

	// Legacy/system (userID==0) → org-wide for read/crawl (writes fail closed at the guard).
	legacy := agent.accountsForActionPlanning(org, 0)
	if !hasAccount(legacy, a1) || !hasAccount(legacy, b1) || !hasAccount(legacy, unassigned) {
		t.Fatalf("legacy path must return org-wide accounts, got %v", accountIDs(legacy))
	}
}

func accountIDs(accs []models.Account) []int64 {
	ids := make([]int64, 0, len(accs))
	for _, a := range accs {
		ids = append(ids, a.ID)
	}
	return ids
}
