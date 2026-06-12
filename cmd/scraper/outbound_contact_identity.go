package main

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// resolveStaffContactIdentity swaps the workspace-default contact for
// the ASSIGNED salesperson's own contact line (PR-5) before comment
// generation. Resolution chain:
//
//	assigned staff contact → company default (only when the workspace
//	allows fallback) → omit the contact line entirely.
//
// The executing account's AssignedUserID names the salesperson; the
// returned identity feeds both the prompt (buildCompanyBlock) and the
// contact guard (ScreenCommentContacts), so a comment can only ever
// cite the contact that was actually resolved here.
func resolveStaffContactIdentity(db *store.Store, orgID, accountID int64, id models.CompanyIdentity) models.CompanyIdentity {
	allowFallback := companyContactFallbackAllowed(db, orgID)
	if accountID <= 0 {
		return models.ApplyStaffContact(nil, id, allowFallback)
	}
	acc, err := db.Identities().GetAccountForOrg(accountID, orgID)
	if err != nil || acc == nil || acc.AssignedUserID <= 0 {
		return models.ApplyStaffContact(nil, id, allowFallback)
	}
	staff, err := db.GetStaffContactProfile(orgID, acc.AssignedUserID)
	if err != nil {
		return models.ApplyStaffContact(nil, id, allowFallback)
	}
	return models.ApplyStaffContact(staff, id, allowFallback)
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
