package models

import "strings"

// StaffContactProfile is one salesperson's own contact identity
// (SaaS UX Hardening PR-5) — distinct from CompanyIdentity (brand /
// service truth). The comment generator cites the ASSIGNED staff's
// contact, so two salespeople produce different contact lines instead
// of every comment reusing one workspace handle.
type StaffContactProfile struct {
	UserID        int64  `json:"user_id"`
	OrgID         int64  `json:"org_id"`
	DisplayName   string `json:"display_name"`
	RoleTitle     string `json:"role_title"`
	Telegram      string `json:"telegram"`
	Zalo          string `json:"zalo"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	PreferredCTA  string `json:"preferred_cta"`
	SignatureText string `json:"signature_text"`
	Visibility    string `json:"visibility"`
	Active        bool   `json:"active"`
}

// ContactLine renders the staff contact as a grounded plain-text label
// ("Telegram @sale · Zalo 09xx"). Empty when no channel is filled —
// callers then fall back (or omit) per workspace policy. The output is
// what CompanyIdentity.OfficialContact carries into the prompt AND what
// the contact screening grounds against, so generation and guard can
// never disagree.
func (p *StaffContactProfile) ContactLine() string {
	if p == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if t := strings.TrimSpace(p.Telegram); t != "" {
		if !strings.HasPrefix(t, "@") && !strings.Contains(t, "/") {
			t = "@" + t
		}
		parts = append(parts, "Telegram "+t)
	}
	if z := strings.TrimSpace(p.Zalo); z != "" {
		parts = append(parts, "Zalo "+z)
	}
	if ph := strings.TrimSpace(p.Phone); ph != "" {
		parts = append(parts, "SĐT "+ph)
	}
	if e := strings.TrimSpace(p.Email); e != "" && len(parts) == 0 {
		parts = append(parts, "Email "+e)
	}
	return strings.Join(parts, " · ")
}

// ApplyStaffContact resolves the contact identity for ONE execution
// (PR-5 resolution chain): assigned staff contact → company default
// (only when allowFallback) → omit entirely (degrade honestly — a
// missing contact is never invented).
//
// PURE. The returned identity feeds buildCompanyBlock (prompt) and
// ScreenCommentContacts (guard) unchanged.
func ApplyStaffContact(staff *StaffContactProfile, id CompanyIdentity, allowFallback bool) CompanyIdentity {
	line := ""
	if staff != nil && staff.Active {
		line = staff.ContactLine()
	}
	switch {
	case line != "":
		id.OfficialContact = line
		if staff.PreferredCTA != "" {
			id.PrimaryCTA = strings.TrimSpace(staff.PreferredCTA)
		}
	case !allowFallback:
		id.OfficialContact = "" // no staff contact + fallback disabled → no contact line at all
	}
	return id
}
