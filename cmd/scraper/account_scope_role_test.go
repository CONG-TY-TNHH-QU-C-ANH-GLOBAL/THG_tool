package main

import "testing"

// callerRestrictedToOwnedAccounts is the single OWNER-scope role classification
// shared by ownedAccountCandidates (outbound) and pickReadyFacebookAccountIDForCrawl
// (crawl) per the approved ARCHCM-R1 Option A decision. This pins the RBAC behavior
// the extraction must preserve: only an identified, non-privileged sales member is
// restricted to owned accounts; admin / platform roles and the userID<=0 scheduler /
// legacy path stay org-wide. A regression here is an account-scope auth change.
func TestCallerRestrictedToOwnedAccounts(t *testing.T) {
	cases := []struct {
		name   string
		userID int64
		role   string
		want   bool
	}{
		// sales member (identified, non-privileged) → restricted to owned accounts.
		{"sales restricted", 7, "sales", true},
		{"sales mixed case + spaces", 7, "  Sales  ", true},
		{"unknown role treated as restricted member", 7, "member", true},
		{"empty role but identified user → restricted", 7, "", true},
		// admin / platform → unrestricted (org-wide).
		{"admin unrestricted", 5, "admin", false},
		{"admin mixed case", 5, "ADMIN", false},
		{"founder unrestricted", 5, "founder", false},
		{"superadmin unrestricted", 5, "superadmin", false},
		// userID <= 0 scheduler / legacy path → unrestricted regardless of role.
		{"scheduler userID 0", 0, "", false},
		{"scheduler userID 0 with sales role", 0, "sales", false},
		{"negative userID", -1, "sales", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := callerRestrictedToOwnedAccounts(tc.userID, tc.role); got != tc.want {
				t.Errorf("callerRestrictedToOwnedAccounts(%d, %q) = %v, want %v", tc.userID, tc.role, got, tc.want)
			}
		})
	}
}
