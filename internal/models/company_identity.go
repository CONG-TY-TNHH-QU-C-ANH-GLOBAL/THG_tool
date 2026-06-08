package models

import "strings"

// CompanyIdentity is the brand-trust projection a grounded comment may use: who
// we are + how to reach us. It is a CONTRACT, not (yet) a stored asset — today it
// is resolved from BusinessProfile + grounded CTA (see ai.ResolveCompanyIdentity);
// later a dedicated `company_identity` KnowledgeOS asset can populate it directly
// without changing this shape or the comment prompt. Every field is grounded
// (empty = not known); the comment generator + quality guard NEVER fabricate a
// website / contact that is not present here.
//
// See specs/OMNICHANNEL_SALES_COPILOT_TELEGRAM_TRACK.md (PR-3).
type CompanyIdentity struct {
	CompanyName    string `json:"company_name"`
	Website        string `json:"website"`         // grounded official URL ("" if unknown)
	OfficialContact string `json:"official_contact"` // grounded Telegram/Zalo/email ("" if unknown)
	PrimaryCTA     string `json:"primary_cta"`     // grounded CTA phrasing ("" → generic "inbox" fallback)
	ServiceSummary string `json:"service_summary"`
}

// HasBrand reports whether there is at least a company name to anchor trust.
func (c CompanyIdentity) HasBrand() bool { return strings.TrimSpace(c.CompanyName) != "" }

// AllowedURL returns the single grounded website the comment may cite (lowercased,
// scheme/trailing-slash-insensitive), or "" when none is grounded.
func (c CompanyIdentity) AllowedURL() string {
	return normalizeURLForMatch(c.Website)
}

// normalizeURLForMatch lowercases + strips scheme, "www.", and trailing slashes so
// a comment URL can be matched against the grounded website tolerantly.
func normalizeURLForMatch(u string) string {
	s := strings.ToLower(strings.TrimSpace(u))
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "www.")
	return strings.TrimRight(s, "/")
}
