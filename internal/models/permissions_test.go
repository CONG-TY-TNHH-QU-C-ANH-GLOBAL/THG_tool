package models

import "testing"

// TestIsAccountOwnerAllowed locks in the canonical execution-layer ownership
// gate (the Organic Sales Network RBAC primitive PR0 relies on): leads are
// shared, but a member may only act on accounts they own — admin/platform
// override; sales must be the assigned owner.
func TestIsAccountOwnerAllowed(t *testing.T) {
	const owner, other int64 = 7, 99
	assigned := &Account{AssignedUserID: owner}
	unassigned := &Account{AssignedUserID: 0}

	cases := []struct {
		name   string
		acc    *Account
		userID int64
		role   string
		want   bool
	}{
		{"nil account blocks everyone (incl admin)", nil, owner, "admin", false},
		{"founder passes any account", assigned, other, "founder", true},
		{"superadmin passes any account", assigned, other, "superadmin", true},
		{"admin passes any account", assigned, other, "admin", true},
		{"admin passes unassigned", unassigned, other, "admin", true},
		{"role case-insensitive", assigned, other, "ADMIN", true},
		{"sales owner passes", assigned, owner, "sales", true},
		{"sales non-owner blocked", assigned, other, "sales", false},
		{"sales blocked on unassigned", unassigned, owner, "sales", false},
		{"empty role treated as non-privileged", assigned, other, "", false},
	}
	for _, tc := range cases {
		if got := IsAccountOwnerAllowed(tc.acc, tc.userID, tc.role); got != tc.want {
			t.Errorf("%s: IsAccountOwnerAllowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRestrictedToOwnedAccounts pins the OWNER-scope role classification shared by the
// outbound candidate pool and crawl auto-pick (ARCHCM2a): only an identified,
// non-privileged sales member is restricted to owned accounts; admin / platform and the
// userID<=0 scheduler / legacy path are org-wide. A regression is an account-scope change.
func TestRestrictedToOwnedAccounts(t *testing.T) {
	cases := []struct {
		name   string
		userID int64
		role   string
		want   bool
	}{
		{"sales restricted", 7, "sales", true},
		{"sales mixed case + spaces", 7, "  Sales  ", true},
		{"unknown role treated as restricted member", 7, "member", true},
		{"empty role but identified user", 7, "", true},
		{"admin unrestricted", 5, "admin", false},
		{"admin mixed case", 5, "ADMIN", false},
		{"founder unrestricted", 5, "founder", false},
		{"superadmin unrestricted", 5, "superadmin", false},
		{"scheduler userID 0", 0, "", false},
		{"scheduler userID 0 with sales role", 0, "sales", false},
		{"negative userID", -1, "sales", false},
	}
	for _, tc := range cases {
		if got := RestrictedToOwnedAccounts(tc.userID, tc.role); got != tc.want {
			t.Errorf("%s: RestrictedToOwnedAccounts(%d, %q) = %v, want %v", tc.name, tc.userID, tc.role, got, tc.want)
		}
	}
}
