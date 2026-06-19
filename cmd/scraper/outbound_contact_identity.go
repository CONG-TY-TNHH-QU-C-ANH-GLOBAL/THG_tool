package main

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// resolveStaffContactIdentity swaps the workspace-default contact for the
// responsible salesperson's own contact line (PR-5; Sprint 5 precedence fix)
// before comment generation. Resolution chain, first usable wins:
//
//	1. the INITIATING sales agent (actx.InitiatorUserID = created_by — the
//	   member handling this outreach; "lead do bạn phụ trách"). Leads are
//	   SHARED with no per-lead owner, so execution ownership names the agent.
//	2. the executing account's ASSIGNED salesperson (acc.AssignedUserID) —
//	   covers automated/scheduled runs with no human initiator contact.
//	3. company default — only when the workspace allows fallback.
//	4. omit the contact line entirely.
//
// A profile is "usable" only when Active with a non-empty ContactLine; an empty
// or inactive profile falls through to the next tier (this is what makes the
// initiator-first rule degrade safely to the assignee for a contactless admin).
// The returned identity feeds both the prompt (buildCompanyBlock) and the
// contact guard (ScreenCommentContacts), so a comment can only ever cite the
// contact that was actually resolved here. Never invents contact data.
func resolveStaffContactIdentity(db *store.Store, orgID, initiatorUserID, accountID int64, id models.CompanyIdentity) models.CompanyIdentity {
	allowFallback := companyContactFallbackAllowed(db, orgID)
	if staff := usableStaffContact(db, orgID, initiatorUserID); staff != nil {
		return models.ApplyStaffContact(staff, id, allowFallback)
	}
	if accountID > 0 {
		if acc, err := db.Identities().GetAccountForOrg(accountID, orgID); err == nil && acc != nil {
			if staff := usableStaffContact(db, orgID, acc.AssignedUserID); staff != nil {
				return models.ApplyStaffContact(staff, id, allowFallback)
			}
		}
	}
	return models.ApplyStaffContact(nil, id, allowFallback)
}

// usableStaffContact returns the org-scoped staff profile for userID ONLY when
// it is selectable for grounding — Active with a non-empty ContactLine. Returns
// nil on any miss (non-positive id, no row, DB error, inactive, or empty) so the
// caller falls through to the next precedence tier. org-scoped: the orgID filter
// keeps a profile from another tenant unreachable.
func usableStaffContact(db *store.Store, orgID, userID int64) *models.StaffContactProfile {
	if userID <= 0 {
		return nil
	}
	staff, err := db.GetStaffContactProfile(orgID, userID)
	if err != nil || staff == nil || !staff.Active || staff.ContactLine() == "" {
		return nil
	}
	return staff
}

// companyContactFallbackAllowed reads the org policy flag
// (default TRUE — workspaces keep today's behavior unless they opt out).
func companyContactFallbackAllowed(db *store.Store, orgID int64) bool {
	v, err := db.Leads().GetContext(fmt.Sprintf("org:%d:allow_company_contact_fallback", orgID))
	if err != nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(v), "false")
}
