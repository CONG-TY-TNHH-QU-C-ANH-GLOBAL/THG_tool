package facebook

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
)

// ContactDirectory is the narrow, consumer-owned port the FB contact-identity resolution
// needs from the data layer. services/facebook owns this interface and depends ONLY on it
// (plus models/ai) — it does not import internal/store. The composition root (cmd/scraper)
// supplies a tiny adapter backed by *store.Store. Methods are tenant-scoped by orgID.
type ContactDirectory interface {
	// StaffContactProfile returns the org-scoped staff profile for userID, or nil if none.
	StaffContactProfile(orgID, userID int64) (*models.StaffContactProfile, error)
	// AccountForOrg returns the account (with its OrgID) or nil if not in this org.
	AccountForOrg(accountID, orgID int64) (*models.Account, error)
	// LeadContext returns a stored context value by key (empty when unset).
	LeadContext(key string) (string, error)
}

// ResolveCommentIdentity builds the SINGLE grounded comment identity that BOTH the normal
// comment path and the reasoning=live path share, so the contact precedence contract lives
// in exactly one place. It seeds the company identity (brand + website + per-lead grounded
// CTA) then applies the staff-contact swap (channels + CTA). groundedCTA is nil on the normal
// path and decision.Selected.CTA on the live path. The company WEBSITE always comes from the
// company identity and is preserved through the staff swap (ApplyStaffContact overrides only
// the contact channels + CTA), so a staff contact never drops the company website.
func ResolveCommentIdentity(dir ContactDirectory, orgID, initiatorUserID, accountID int64, profile *ai.BusinessProfile, groundedCTA *models.GroundedItem) models.CompanyIdentity {
	id := ai.ResolveCompanyIdentity(profile, groundedCTA)
	return resolveStaffContactIdentity(dir, orgID, initiatorUserID, accountID, id)
}

// resolveStaffContactIdentity swaps the workspace-default contact for the responsible
// salesperson's own contact line (PR-5; Sprint 5 precedence fix) before comment generation.
// Resolution chain, first usable wins:
//
//  1. the INITIATING sales agent (actx.InitiatorUserID = created_by — the member handling
//     this outreach; "lead do bạn phụ trách"). Leads are SHARED with no per-lead owner, so
//     execution ownership names the agent.
//  2. the executing account's ASSIGNED salesperson (acc.AssignedUserID) — covers
//     automated/scheduled runs with no human initiator contact.
//  3. company default — only when the workspace allows fallback.
//  4. omit the contact line entirely.
//
// A profile is "usable" only when Active with a non-empty ContactLine; an empty or inactive
// profile falls through to the next tier. The returned identity feeds both the prompt
// (buildCompanyBlock) and the contact guard (ScreenCommentContacts), so a comment can only
// ever cite the contact resolved here. Never invents contact data.
func resolveStaffContactIdentity(dir ContactDirectory, orgID, initiatorUserID, accountID int64, id models.CompanyIdentity) models.CompanyIdentity {
	allowFallback := companyContactFallbackAllowed(dir, orgID)
	if staff := usableStaffContact(dir, orgID, initiatorUserID); staff != nil {
		return models.ApplyStaffContact(staff, id, allowFallback)
	}
	if accountID > 0 {
		if acc, err := dir.AccountForOrg(accountID, orgID); err == nil && acc != nil {
			if staff := usableStaffContact(dir, orgID, acc.AssignedUserID); staff != nil {
				return models.ApplyStaffContact(staff, id, allowFallback)
			}
		}
	}
	return models.ApplyStaffContact(nil, id, allowFallback)
}

// usableStaffContact returns the org-scoped staff profile for userID ONLY when it is
// selectable for grounding — Active with a non-empty ContactLine. Returns nil on any miss
// (non-positive id, no row, error, inactive, or empty) so the caller falls through to the
// next precedence tier. org-scoped: the orgID keeps another tenant's profile unreachable.
func usableStaffContact(dir ContactDirectory, orgID, userID int64) *models.StaffContactProfile {
	if userID <= 0 {
		return nil
	}
	staff, err := dir.StaffContactProfile(orgID, userID)
	if err != nil || staff == nil || !staff.Active || staff.ContactLine() == "" {
		return nil
	}
	return staff
}

// companyContactFallbackAllowed reads the org policy flag (default TRUE — workspaces keep
// today's behavior unless they opt out).
func companyContactFallbackAllowed(dir ContactDirectory, orgID int64) bool {
	v, err := dir.LeadContext(fmt.Sprintf("org:%d:allow_company_contact_fallback", orgID))
	if err != nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(v), "false")
}
