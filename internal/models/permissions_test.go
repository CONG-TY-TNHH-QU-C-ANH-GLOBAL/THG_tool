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
