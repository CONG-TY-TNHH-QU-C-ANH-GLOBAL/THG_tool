package models

import "strings"

// IsAccountOwnerAllowed is the canonical execution-layer permission check.
//
// Battlefield-model rule (see feedback_shared_battlefield_not_crm.md):
//   - Platform roles (founder / superadmin) always pass.
//   - Admin always passes (admin override).
//   - Sales must be the account's AssignedUserID.
//   - Nil account → block (including admin — there is nothing to operate on).
//   - Unassigned account (AssignedUserID == 0) → only admin passes.
//
// Pure logic; no DB access. Lives in models so HTTP handlers, store helpers,
// and skill executors can all call into the same gate without circular imports.
func IsAccountOwnerAllowed(acc *Account, userID int64, role string) bool {
	if acc == nil {
		return false
	}
	r := UserRole(strings.TrimSpace(strings.ToLower(role)))
	if IsPlatformRole(r) || r == RoleAdmin {
		return true
	}
	return acc.AssignedUserID > 0 && acc.AssignedUserID == userID
}

// CanViewAccountDevice is the VIEW-side privacy gate for a Facebook account /
// device identity + live session (PR-M5). It is STRICTER than
// IsAccountOwnerAllowed: a Facebook account belongs to the member who owns it
// and is PRIVATE to them — even an admin cannot view a staff member's account.
// Admin oversight of staff is the per-staff automation activity (comments /
// posts / inbox) + online status on the Nhân viên tab, never the account itself.
//
//   - Account assigned to the caller          → visible.
//   - Unassigned account (AssignedUserID == 0) → visible to admin/platform only
//     (org-owned but unclaimed — admin may still manage it).
//   - Account assigned to ANOTHER member       → hidden from EVERYONE, admin incl.
func CanViewAccountDevice(acc *Account, userID int64, role string) bool {
	if acc == nil {
		return false
	}
	if acc.AssignedUserID > 0 && acc.AssignedUserID == userID {
		return true
	}
	r := UserRole(strings.TrimSpace(strings.ToLower(role)))
	return acc.AssignedUserID == 0 && (IsPlatformRole(r) || r == RoleAdmin)
}

// AccountControlAllowed is the EXECUTION-control predicate for the readiness UI (P1.3E),
// deliberately stricter than CanViewAccountDevice: VISIBILITY is not CONTROL. An account is
// controllable by a requester only when it is ASSIGNED to them — admin role and inventory
// visibility grant NOTHING here, so an admin viewing an unassigned/member account does not
// control it. requesterUserID <= 0 (unauthenticated) controls nothing. It is intentionally
// no looser than the shipped P1.3D execution gate (own connector + own/unassigned account):
// the readiness UI may only ever UNDER-promise executability, never over-promise it.
func AccountControlAllowed(acc *Account, requesterUserID int64) bool {
	if acc == nil || requesterUserID <= 0 {
		return false
	}
	return acc.AssignedUserID == requesterUserID
}
