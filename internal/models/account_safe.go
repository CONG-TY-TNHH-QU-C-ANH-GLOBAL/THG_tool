package models

import "time"

// AccountSafe is the HTTP/API projection of Account. It is a contract
// boundary, not a DB row serialization: handlers must NEVER marshal
// models.Account into a response, because Account carries decrypted
// credentials after the store layer (CookiesJSON via auth.Decrypt) plus
// infrastructure fields (ProxyURL, UserAgent) that classify as secrets.
//
// Explicitly excluded — adding any of these back is a security
// regression (see specs/domains/platform-foundation/features/workspace-ui/implementation/ux-hardening-track.md PR-2):
//   - CookiesJSON (decrypted Facebook session cookies)
//   - ProxyURL (proxy infrastructure, may embed credentials)
//   - UserAgent (browser fingerprint material)
//
// Internal workers keep using the full Account model; only the HTTP
// layer projects through AccountSafe.
type AccountSafe struct {
	ID               int64         `json:"id"`
	OrgID            int64         `json:"org_id"`
	Platform         Platform      `json:"platform"`
	Name             string        `json:"name"`
	Email            string        `json:"email"`
	Status           AccountStatus `json:"status"`
	Notes            string        `json:"notes,omitempty"`
	LastUsed         time.Time     `json:"last_used"`
	CreatedAt        time.Time     `json:"created_at"`
	AssignedUserID   int64         `json:"assigned_user_id"`
	AssignedUserName string        `json:"assigned_user_name"`
	BrowserLoggedIn  bool          `json:"browser_logged_in"`
	FBUserID         string        `json:"fb_user_id"`
	FBDisplayName    string        `json:"fb_display_name"`
	FBUsername       string        `json:"fb_username"`
	FBProfileURL     string        `json:"fb_profile_url"`
}

// NewAccountSafe projects one Account into its safe HTTP shape.
func NewAccountSafe(a *Account) AccountSafe {
	return AccountSafe{
		ID:               a.ID,
		OrgID:            a.OrgID,
		Platform:         a.Platform,
		Name:             a.Name,
		Email:            a.Email,
		Status:           a.Status,
		Notes:            a.Notes,
		LastUsed:         a.LastUsed,
		CreatedAt:        a.CreatedAt,
		AssignedUserID:   a.AssignedUserID,
		AssignedUserName: a.AssignedUserName,
		BrowserLoggedIn:  a.BrowserLoggedIn,
		FBUserID:         a.FBUserID,
		FBDisplayName:    a.FBDisplayName,
		FBUsername:       a.FBUsername,
		FBProfileURL:     a.FBProfileURL,
	}
}

// AccountSafeList projects a slice of Accounts for list endpoints.
// Always returns a non-nil slice so JSON renders [] instead of null.
func AccountSafeList(accs []Account) []AccountSafe {
	out := make([]AccountSafe, 0, len(accs))
	for i := range accs {
		out = append(out, NewAccountSafe(&accs[i]))
	}
	return out
}
