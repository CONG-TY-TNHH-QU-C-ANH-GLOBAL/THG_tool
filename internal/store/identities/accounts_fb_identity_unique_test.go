package identities_test

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// TestAccountFacebookIdentityUnique locks in the PR1 invariant: one Facebook
// identity = one account per org (partial UNIQUE(org_id, fb_user_id)). This is
// what makes ResolveOrCreateAccountForFacebookIdentity (PR2) race-safe.
func TestAccountFacebookIdentityUnique(t *testing.T) {
	_, ids := newIdentitiesStore(t, "fb-identity-unique.db")
	const orgA, orgB int64 = 1, 2

	mk := func(org int64, name string) int64 {
		id, err := ids.AddAccount(&models.Account{
			OrgID: org, Platform: models.PlatformFacebook, Name: name, Status: models.AccountActive,
		})
		if err != nil {
			t.Fatalf("AddAccount(%s): %v", name, err)
		}
		return id
	}

	// Two fresh slots (empty fb_user_id) coexist — partial index excludes ''.
	a1 := mk(orgA, "A1")
	a2 := mk(orgA, "A2")

	// Bind a1 to FB-X (ok).
	if err := ids.SetAccountFacebookIdentity(a1, "fbX", ""); err != nil {
		t.Fatalf("bind a1->fbX: %v", err)
	}
	// Binding a SECOND account in the SAME org to the SAME FB identity must be
	// rejected by the unique index (the account-slot mismatch guard does not
	// catch this — a2's slot is empty — so the DB constraint is the backstop).
	if err := ids.SetAccountFacebookIdentity(a2, "fbX", ""); err == nil {
		t.Fatal("expected unique violation binding a 2nd account to the same FB identity in one org")
	}

	// Same FB identity in a DIFFERENT org is allowed (org-scoped uniqueness).
	b1 := mk(orgB, "B1")
	if err := ids.SetAccountFacebookIdentity(b1, "fbX", ""); err != nil {
		t.Fatalf("bind b1->fbX in orgB should be allowed: %v", err)
	}
}
