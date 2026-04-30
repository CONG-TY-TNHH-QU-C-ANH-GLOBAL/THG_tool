package models

import "time"

// PlanTier defines the subscription level for an organization.
type PlanTier string

const (
	PlanFree       PlanTier = "free"
	PlanPro        PlanTier = "pro"
	PlanEnterprise PlanTier = "enterprise"
)

// Organization is a client tenant — one per business using the platform.
// All data (accounts, groups, leads, etc.) is isolated per organization.
type Organization struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Domain      string    `json:"domain"`       // e.g., thgfulfill.com
	PlanTier    PlanTier  `json:"plan_tier"`    // free | pro | enterprise
	MaxAccounts int       `json:"max_accounts"` // 0 = unlimited
	Abbr        string    `json:"abbr"`
	Color       string    `json:"color"`
	LogoURL     string    `json:"logo_url"`
	AvatarURL   string    `json:"avatar_url"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

// PlanLimits returns the default limits for each plan tier.
func (p PlanTier) MaxAccounts() int {
	switch p {
	case PlanFree:
		return 1
	case PlanPro:
		return 5
	case PlanEnterprise:
		return 0 // unlimited
	}
	return 1
}
