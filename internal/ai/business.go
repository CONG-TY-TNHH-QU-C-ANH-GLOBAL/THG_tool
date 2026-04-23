package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/store"
)

// BusinessProfile is the single source of truth for what a customer's business does.
// It replaces all hardcoded "logistics" / "tuyen_dung" logic.
// Stored as individual keys in user_context so legacy set_context calls still work.
type BusinessProfile struct {
	Name        string `json:"name"`         // e.g., "Tiệm Bánh ABC"
	Industry    string `json:"industry"`     // e.g., "food & beverage", "real estate", "recruitment"
	Description string `json:"description"`  // what they do, free-form
	Services    string `json:"services"`     // what they sell/offer
	Targets     string `json:"targets"`      // who they're looking for / ideal customer
	Location    string `json:"location"`     // where they operate
	USP         string `json:"usp"`          // unique selling point
	RejectRules string `json:"reject_rules"` // what posts to ignore
}

// IsConfigured returns true if the profile has enough info for AI operations.
func (p *BusinessProfile) IsConfigured() bool {
	return p != nil && (p.Description != "" || p.Industry != "" || p.Services != "")
}

// ToPromptBlock returns a structured block for injection into any AI prompt.
// Works for ANY industry — no hardcoded business logic.
func (p *BusinessProfile) ToPromptBlock() string {
	if p == nil || !p.IsConfigured() {
		return "(Business profile not configured — user should describe their business first)"
	}
	var sb strings.Builder
	if p.Name != "" {
		fmt.Fprintf(&sb, "BUSINESS: %s\n", p.Name)
	}
	if p.Industry != "" {
		fmt.Fprintf(&sb, "INDUSTRY: %s\n", p.Industry)
	}
	if p.Description != "" {
		fmt.Fprintf(&sb, "WHAT WE DO: %s\n", p.Description)
	}
	if p.Services != "" {
		fmt.Fprintf(&sb, "PRODUCTS/SERVICES: %s\n", p.Services)
	}
	if p.Targets != "" {
		fmt.Fprintf(&sb, "IDEAL CUSTOMER: %s\n", p.Targets)
	}
	if p.Location != "" {
		fmt.Fprintf(&sb, "LOCATION: %s\n", p.Location)
	}
	if p.USP != "" {
		fmt.Fprintf(&sb, "WHY CHOOSE US: %s\n", p.USP)
	}
	if p.RejectRules != "" {
		fmt.Fprintf(&sb, "IGNORE THESE POSTS: %s\n", p.RejectRules)
	}
	return sb.String()
}

// LoadProfile builds a BusinessProfile from the user_context table.
// Reads both new keys (business_industry, business_usp) and legacy keys for backward compat.
func LoadProfile(db *store.Store) *BusinessProfile {
	ctx, err := db.GetAllContext()
	if err != nil {
		return &BusinessProfile{}
	}
	return ProfileFromContext(ctx)
}

// ProfileFromContext builds a BusinessProfile from a context map (works without DB).
func ProfileFromContext(ctx map[string]string) *BusinessProfile {
	return &BusinessProfile{
		Name:        ctx["business_name"],
		Industry:    orFallback(ctx["business_industry"], ctx["active_niche"]),
		Description: ctx["business_desc"],
		Services:    ctx["services"],
		Targets:     ctx["target_customers"],
		Location:    ctx["business_location"],
		USP:         ctx["business_usp"],
		RejectRules: ctx["reject_rules"],
	}
}

// Save persists the profile to user_context as individual keys.
func (p *BusinessProfile) Save(db *store.Store) error {
	fields := map[string]string{
		"business_name":     p.Name,
		"business_industry": p.Industry,
		"business_desc":     p.Description,
		"services":          p.Services,
		"target_customers":  p.Targets,
		"business_location": p.Location,
		"business_usp":      p.USP,
		"reject_rules":      p.RejectRules,
	}
	for k, v := range fields {
		if v == "" {
			continue
		}
		if err := db.SetContext(k, v); err != nil {
			return err
		}
	}
	return nil
}

// ExtractProfile uses AI to extract a structured BusinessProfile from any free-form description.
// This is the onboarding function — user says "Tôi bán bánh mì tại HCM" → structured profile.
func (mg *MessageGenerator) ExtractProfile(ctx context.Context, userDescription string) (*BusinessProfile, error) {
	prompt := fmt.Sprintf(`You are an expert business analyst. Extract a structured business profile from the description below.

USER DESCRIPTION:
"%s"

Respond with ONLY valid JSON (no markdown, no explanation):
{
  "name": "business name if mentioned, else empty",
  "industry": "concise industry label in English (e.g. food_beverage, real_estate, recruitment, logistics, beauty, education, healthcare, ecommerce, technology, finance, retail, other)",
  "description": "what the business does (1-2 sentences, keep user's language)",
  "services": "products/services offered (comma-separated, keep user's language)",
  "targets": "ideal customer profile (who they want to reach, keep user's language)",
  "location": "city/region if mentioned, else empty",
  "usp": "unique selling point if mentioned, else empty",
  "reject_rules": "types of posts to ignore based on their business, else empty"
}`, userDescription)

	raw, err := mg.callOpenAI(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("extract profile: %w", err)
	}

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}") + 1
	if start < 0 || end <= start {
		return nil, fmt.Errorf("bad profile JSON: %s", sliceStr(raw, 100))
	}

	var p BusinessProfile
	if err := json.Unmarshal([]byte(raw[start:end]), &p); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	return &p, nil
}

func orFallback(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
