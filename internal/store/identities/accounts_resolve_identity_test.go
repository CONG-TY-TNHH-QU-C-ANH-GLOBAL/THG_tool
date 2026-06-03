package identities_test

import (
	"testing"

	"github.com/thg/scraper/internal/store/identities"
)

// TestResolveOrCreateAccountForFacebookIdentity locks in the PR2 core primitive:
// each distinct Facebook login becomes its own account, owned by the pairing
// member, created on first sight, and NEVER re-owned on later sightings (no-steal).
func TestResolveOrCreateAccountForFacebookIdentity(t *testing.T) {
	_, ids := newIdentitiesStore(t, "resolve-create.db")
	const org, m1, m2 int64 = 1, 10, 20
	meta := identities.FacebookIdentityMeta{DisplayName: "Alice"}

	// First sight of fbA → create, owned by m1.
	a, created, err := ids.ResolveOrCreateAccountForFacebookIdentity(org, m1, "fbA", meta, "alice@example.com")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if !created || a == nil {
		t.Fatalf("first sight should create: created=%v acc=%v", created, a)
	}
	if a.AssignedUserID != m1 {
		t.Fatalf("owner = %d, want m1=%d", a.AssignedUserID, m1)
	}
	if a.FBUserID != "fbA" {
		t.Fatalf("fb_user_id = %q, want fbA", a.FBUserID)
	}

	// Second sight of fbA — even with a DIFFERENT member passed — returns the same
	// account, does NOT recreate, and does NOT change the owner (no-steal).
	a2, created2, err := ids.ResolveOrCreateAccountForFacebookIdentity(org, m2, "fbA", meta, "")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if created2 {
		t.Fatal("second sight must not create a new account")
	}
	if a2.ID != a.ID || a2.AssignedUserID != m1 {
		t.Fatalf("no-steal violated: id=%d (want %d) owner=%d (want %d)", a2.ID, a.ID, a2.AssignedUserID, m1)
	}

	// A different FB identity for the same member → its own new account.
	b, createdB, err := ids.ResolveOrCreateAccountForFacebookIdentity(org, m1, "fbB", meta, "")
	if err != nil || !createdB || b.ID == a.ID {
		t.Fatalf("fbB should be a new account: created=%v id=%d (a.id=%d) err=%v", createdB, b.ID, a.ID, err)
	}

	// Empty FB identity is a no-op.
	n, c0, err := ids.ResolveOrCreateAccountForFacebookIdentity(org, m1, "", meta, "")
	if err != nil || n != nil || c0 {
		t.Fatalf("empty fb_user_id must be a no-op: acc=%v created=%v err=%v", n, c0, err)
	}

	// GetAccountByFacebookIdentity round-trips.
	got, err := ids.GetAccountByFacebookIdentity(org, "fbA")
	if err != nil || got == nil || got.ID != a.ID {
		t.Fatalf("GetAccountByFacebookIdentity(fbA) = %v err=%v, want id %d", got, err, a.ID)
	}
	if miss, _ := ids.GetAccountByFacebookIdentity(org, "nope"); miss != nil {
		t.Fatal("unknown fb identity should return nil")
	}
}
