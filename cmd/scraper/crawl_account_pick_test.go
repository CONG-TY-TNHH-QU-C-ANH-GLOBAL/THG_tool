package main

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// requireGate asserts the crawlOwnershipGate result: a non-nil gate whose allow(a)/
// allow(b) match the expected ownership decision.
func requireGate(t *testing.T, label string, allow func(int64) bool, err error, a, b int64, wantA, wantB bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected err %v", label, err)
	}
	if allow == nil {
		t.Fatalf("%s: got nil gate, want non-nil", label)
	}
	if allow(a) != wantA {
		t.Errorf("%s: allow(%d)=%v, want %v", label, a, allow(a), wantA)
	}
	if allow(b) != wantB {
		t.Errorf("%s: allow(%d)=%v, want %v", label, b, allow(b), wantB)
	}
}

// crawlOwnershipGate is the PR-M3 auto-pick owner filter (ARCHCM4 invariant #6). This
// pins it across the ARCHCM4a extraction: a non-privileged sales member is limited to
// accounts they own; admin / platform / the userID<=0 scheduler are org-wide; a member
// who owns nothing yields a nil gate (caller picks nothing). A regression here is an
// account-scope auth change.
func TestCrawlOwnershipGate(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "gate.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	a, _ := db.Identities().AddAccount(&models.Account{OrgID: 1, Platform: models.PlatformFacebook, Name: "Alice", AssignedUserID: 7, Status: models.AccountActive})
	b, _ := db.Identities().AddAccount(&models.Account{OrgID: 1, Platform: models.PlatformFacebook, Name: "Bob", AssignedUserID: 8, Status: models.AccountActive})

	// sales is restricted to owned accounts.
	allow, gErr := crawlOwnershipGate(db, 1, 7, "sales")
	requireGate(t, "sales restricted", allow, gErr, a, b, true, false)

	// admin + platform + the userID<=0 scheduler are org-wide.
	allow, gErr = crawlOwnershipGate(db, 1, 5, "admin")
	requireGate(t, "admin org-wide", allow, gErr, a, b, true, true)
	allow, gErr = crawlOwnershipGate(db, 1, 0, "")
	requireGate(t, "scheduler org-wide", allow, gErr, a, b, true, true)

	// a sales member who owns nothing yields a nil gate (pick nothing), not an error.
	allow, gErr = crawlOwnershipGate(db, 1, 99, "sales")
	if gErr != nil || allow != nil {
		t.Errorf("owns nothing: got gate=%v err=%v, want nil gate, nil err", allow != nil, gErr)
	}
}
