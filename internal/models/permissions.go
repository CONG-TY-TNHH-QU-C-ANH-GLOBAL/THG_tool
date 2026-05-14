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
