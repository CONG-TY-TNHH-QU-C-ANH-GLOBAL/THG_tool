package store

import (
	"errors"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Staff removal != user deletion: detach resets the membership ONLY —
// the global user row, credentials, and the ability to join another
// workspace all survive; the leaver's accounts are assignment-paused.
func TestDetachUserFromOrg_PreservesIdentity(t *testing.T) {
	db := newSharedStore(t, "detach.db")
	adminID, _ := db.CreateUser(&models.User{OrgID: 5, Email: "boss@example.com", Name: "Boss", PasswordHash: "hash-boss", Role: models.RoleAdmin})
	salesID, _ := db.CreateUser(&models.User{OrgID: 5, Email: "sale@example.com", Name: "Sale", PasswordHash: "hash-sale", Role: models.RoleSales})
	accID, _ := db.Identities().AddAccount(&models.Account{
		OrgID: 5, Platform: models.PlatformFacebook, Name: "Sale FB",
		AssignedUserID: salesID, Status: models.AccountActive,
	})
	_ = adminID

	if err := db.DetachUserFromOrg(salesID, 5); err != nil {
		t.Fatalf("detach: %v", err)
	}

	// Identity + credentials preserved; membership reset to orgless default.
	u, err := db.GetUserByID(salesID)
	if err != nil || u == nil {
		t.Fatalf("user row destroyed by detach: %v", err)
	}
	if u.OrgID != 0 || u.Role != models.RoleAdmin {
		t.Fatalf("detach state: org=%d role=%s, want org=0 role=admin", u.OrgID, u.Role)
	}
	var hash string
	_ = db.DB().QueryRow(`SELECT password_hash FROM users WHERE id = ?`, salesID).Scan(&hash)
	if hash != "hash-sale" {
		t.Fatalf("login credentials must survive detach, got %q", hash)
	}
	// Automation safety: the leaver's accounts are paused.
	if paused, _ := db.Identities().AccountAssignmentPaused(accID); !paused {
		t.Fatalf("leaver's account must be assignment-paused")
	}
}

// The last active admin can never be removed or leave.
func TestDetachUserFromOrg_LastAdminGuard(t *testing.T) {
	db := newSharedStore(t, "detach_last_admin.db")
	soloAdmin, _ := db.CreateUser(&models.User{OrgID: 6, Email: "solo@example.com", Name: "Solo", PasswordHash: "x", Role: models.RoleAdmin})
	_, _ = db.CreateUser(&models.User{OrgID: 6, Email: "s@example.com", Name: "S", PasswordHash: "x", Role: models.RoleSales})

	if err := db.DetachUserFromOrg(soloAdmin, 6); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("last admin detach = %v, want ErrLastAdmin", err)
	}
	if u, _ := db.GetUserByID(soloAdmin); u.OrgID != 6 {
		t.Fatalf("blocked detach must not mutate membership")
	}

	// A second admin unblocks the guard.
	secondAdmin, _ := db.CreateUser(&models.User{OrgID: 6, Email: "a2@example.com", Name: "A2", PasswordHash: "x", Role: models.RoleAdmin})
	_ = secondAdmin
	if err := db.DetachUserFromOrg(soloAdmin, 6); err != nil {
		t.Fatalf("detach with second admin present: %v", err)
	}

	// Non-members cannot be detached.
	if err := db.DetachUserFromOrg(soloAdmin, 6); err == nil {
		t.Fatalf("detaching a non-member must fail")
	}
}

// Pending invite != membership: a pending invite must NOT be a
// provisioned-org claim — signup/login never auto-joins; only the
// explicit accept endpoint grants membership.
func TestFindProvisionedOrgByEmail_PendingInviteIsNotAClaim(t *testing.T) {
	db := newSharedStore(t, "invite_not_claim.db")
	orgID, _ := db.CreateOrganization(&models.Organization{Name: "Org", PlanTier: models.PlanFree, Active: true})
	if _, err := db.DB().Exec(
		`INSERT INTO org_invites (org_id, email, role, token, created_by, expires_at)
		 VALUES (?, 'invitee@example.com', 'sales', 'tok-claim-test', 1, datetime('now', '+7 days'))`,
		orgID,
	); err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	claim, err := db.FindProvisionedOrgByEmail("invitee@example.com")
	if err != nil {
		t.Fatalf("FindProvisionedOrgByEmail: %v", err)
	}
	if claim != nil {
		t.Fatalf("pending invite produced a provisioned claim %+v — membership would be granted without accept", claim)
	}
}
