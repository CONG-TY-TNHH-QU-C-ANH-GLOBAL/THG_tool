package agent

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// AccountOwnerAllowed implements the battlefield-model execution gate:
// admin / platform = always pass; sales = must be assigned owner.
// See feedback_shared_battlefield_not_crm.md.
func TestAccountOwnerAllowed(t *testing.T) {
	acc := &models.Account{ID: 10, AssignedUserID: 7}

	cases := []struct {
		name   string
		userID int64
		role   string
		want   bool
	}{
		{"admin passes regardless of ownership", 99, string(models.RoleAdmin), true},
		{"admin uppercase still passes", 99, "ADMIN", true},
		{"founder always passes", 99, string(models.RoleFounder), true},
		{"superadmin always passes", 99, string(models.RoleSuperAdmin), true},
		{"sales owner allowed", 7, string(models.RoleSales), true},
		{"sales non-owner blocked", 8, string(models.RoleSales), false},
		{"sales when account has no owner blocked", 8, string(models.RoleSales), false},
		{"empty role treated as sales (strict)", 7, "", true},
		{"empty role non-owner blocked", 8, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AccountOwnerAllowed(acc, tc.userID, tc.role)
			if got != tc.want {
				t.Errorf("AccountOwnerAllowed(userID=%d, role=%q) = %v, want %v",
					tc.userID, tc.role, got, tc.want)
			}
		})
	}
}

func TestAccountOwnerAllowed_NilAccount(t *testing.T) {
	if AccountOwnerAllowed(nil, 1, string(models.RoleAdmin)) {
		t.Error("nil account must not pass even for admin")
	}
}

func TestAccountOwnerAllowed_UnassignedAccountStrict(t *testing.T) {
	acc := &models.Account{ID: 10, AssignedUserID: 0}
	if AccountOwnerAllowed(acc, 7, string(models.RoleSales)) {
		t.Error("unassigned account (AssignedUserID=0) must block sales staff")
	}
	if !AccountOwnerAllowed(acc, 7, string(models.RoleAdmin)) {
		t.Error("unassigned account must still allow admin")
	}
}
