package main

import (
	"strings"

	"github.com/thg/scraper/internal/models"
)

// callerRestrictedToOwnedAccounts reports whether the caller may only resolve /
// auto-pick Facebook accounts they personally own. This is the canonical OWNER-scope
// role classification extracted per the approved ARCHCM-R1 Option A decision so the
// rule lives in exactly one place.
//
// ownedAccountCandidates (outbound) uses it today. The crawl ownership gate
// (pickReadyFacebookAccountIDForCrawl) re-implements the same rule inline and will
// adopt this helper under ARCHCM4 — that function is RED-adjacent crawl runtime over
// the cognitive-complexity threshold and is gated behind the ARCHCM-R2 audit, so it
// is deliberately left untouched here.
//
// Restricted  → identified sales member (userID > 0, non-privileged): owned accounts only.
// Unrestricted → admin / platform roles, AND the userID <= 0 scheduler / legacy path:
//                org-wide (NOT enumerated — the consumer's own gate stays permissive).
//
// It decides only WHETHER the owned restriction applies; it does NOT decide
// per-account ownership (that is models.IsAccountOwnerAllowed) and is unrelated to
// the role-blind CONTROL gate (canRequesterControlAccount) or VISIBILITY
// (models.CanViewAccountDevice). The three gates stay distinct (ARCHCM-R1).
func callerRestrictedToOwnedAccounts(userID int64, role string) bool {
	if userID <= 0 {
		return false
	}
	r := models.UserRole(strings.ToLower(strings.TrimSpace(role)))
	return !models.IsPlatformRole(r) && r != models.RoleAdmin
}
