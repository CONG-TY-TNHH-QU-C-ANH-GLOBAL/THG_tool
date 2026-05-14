package models

import (
	"encoding/json"
	"strings"
	"time"
)

// BehaviourCaps is the resolved runtime policy for one account: the daily
// limits the queue layer enforces and the cooldown the orchestrator uses
// when spacing actions. Caps are DERIVED from (TrustLevel, WorkspaceRole)
// by ResolveBehaviourCaps; individual accounts do not carry numeric caps.
//
// Per-account exceptions are handled by AccountBehaviourProfile.CapsOverride
// — applied on top of the resolved caps via OverlayCaps.
type BehaviourCaps struct {
	// Daily caps (counters reset at UTC midnight).
	CommentsPerDay     int `json:"comments_per_day"`
	InboxPerDay        int `json:"inbox_per_day"`
	GroupPostsPerDay   int `json:"group_posts_per_day"`
	ProfilePostsPerDay int `json:"profile_posts_per_day"`

	// Minimum cooldown the orchestrator applies between two actions of
	// the same type on the same scope (group / post / profile). The
	// queue-time layer enforces only daily counters; contextual cooldowns
	// are the orchestrator's job once it reads the Action Ledger.
	SameGroupCooldown   time.Duration `json:"same_group_cooldown_ns"`
	SamePostCooldown    time.Duration `json:"same_post_cooldown_ns"`
	SameProfileCooldown time.Duration `json:"same_profile_cooldown_ns"`

	// Account-wide cooldown applied after any action (anti-burst).
	GlobalActionCooldown time.Duration `json:"global_action_cooldown_ns"`

	// Hard ceiling on risk_score above which the account is blocked from
	// all actions until risk decays. 0 = no ceiling enforced.
	RiskScoreCeiling float64 `json:"risk_score_ceiling"`
}

// CapForAction returns the daily cap for an action type. Unknown types
// return 0 — interpreted by the queue layer as "no cap enforced via the
// daily counter mechanism" (other guards still apply).
func (c BehaviourCaps) CapForAction(action string) int {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "comment":
		return c.CommentsPerDay
	case "inbox":
		return c.InboxPerDay
	case "group_post":
		return c.GroupPostsPerDay
	case "profile_post":
		return c.ProfilePostsPerDay
	default:
		return 0
	}
}

// trustLevelCaps is the canonical preset table. ResolveBehaviourCaps
// reads this and then applies an (optional) workspace-role multiplier.
// Numbers are conservative defaults; the user/admin layer can tune per
// account via CapsOverride.
var trustLevelCaps = map[TrustLevel]BehaviourCaps{
	TrustCold: {
		CommentsPerDay:       3,
		InboxPerDay:          1,
		GroupPostsPerDay:     0,
		ProfilePostsPerDay:   0,
		SameGroupCooldown:    6 * time.Hour,
		SamePostCooldown:     24 * time.Hour,
		SameProfileCooldown:  48 * time.Hour,
		GlobalActionCooldown: 30 * time.Minute,
		RiskScoreCeiling:     0.40,
	},
	TrustWarming: {
		CommentsPerDay:       10,
		InboxPerDay:          5,
		GroupPostsPerDay:     1,
		ProfilePostsPerDay:   1,
		SameGroupCooldown:    3 * time.Hour,
		SamePostCooldown:     12 * time.Hour,
		SameProfileCooldown:  24 * time.Hour,
		GlobalActionCooldown: 10 * time.Minute,
		RiskScoreCeiling:     0.60,
	},
	TrustWarm: {
		CommentsPerDay:       30,
		InboxPerDay:          15,
		GroupPostsPerDay:     3,
		ProfilePostsPerDay:   2,
		SameGroupCooldown:    1 * time.Hour,
		SamePostCooldown:     6 * time.Hour,
		SameProfileCooldown:  12 * time.Hour,
		GlobalActionCooldown: 3 * time.Minute,
		RiskScoreCeiling:     0.75,
	},
	TrustTrusted: {
		CommentsPerDay:       80,
		InboxPerDay:          40,
		GroupPostsPerDay:     8,
		ProfilePostsPerDay:   5,
		SameGroupCooldown:    20 * time.Minute,
		SamePostCooldown:     2 * time.Hour,
		SameProfileCooldown:  6 * time.Hour,
		GlobalActionCooldown: 60 * time.Second,
		RiskScoreCeiling:     0.90,
	},
	TrustSacrificial: {
		CommentsPerDay:       200,
		InboxPerDay:          80,
		GroupPostsPerDay:     20,
		ProfilePostsPerDay:   10,
		SameGroupCooldown:    5 * time.Minute,
		SamePostCooldown:     30 * time.Minute,
		SameProfileCooldown:  2 * time.Hour,
		GlobalActionCooldown: 15 * time.Second,
		RiskScoreCeiling:     0.99,
	},
}

// workspaceRoleMultipliers tilts the resolved caps for behavioural roles.
// Multipliers apply to the daily caps only — cooldowns and risk ceiling
// remain trust-driven. Unknown roles fall through with multiplier 1.0.
var workspaceRoleMultipliers = map[WorkspaceRole]float64{
	"aggressive_outreach": 1.5,
	"passive_engagement":  0.5,
	"inbox_only":          1.0, // see ResolveBehaviourCaps for the inbox-only zeroing rule
}

// ResolveBehaviourCaps returns the runtime caps for a (trust, role) pair.
// Pure function. Unknown trust falls back to TrustWarming. Per-account
// caps_override is applied separately via OverlayCaps so the resolver
// remains stateless.
func ResolveBehaviourCaps(trust TrustLevel, role WorkspaceRole) BehaviourCaps {
	base, ok := trustLevelCaps[trust]
	if !ok {
		base = trustLevelCaps[TrustWarming]
	}
	mult := 1.0
	if m, ok := workspaceRoleMultipliers[WorkspaceRole(strings.ToLower(strings.TrimSpace(string(role))))]; ok {
		mult = m
	}
	out := base
	out.CommentsPerDay = scaleCap(base.CommentsPerDay, mult)
	out.InboxPerDay = scaleCap(base.InboxPerDay, mult)
	out.GroupPostsPerDay = scaleCap(base.GroupPostsPerDay, mult)
	out.ProfilePostsPerDay = scaleCap(base.ProfilePostsPerDay, mult)
	if WorkspaceRole(strings.ToLower(strings.TrimSpace(string(role)))) == "inbox_only" {
		out.CommentsPerDay = 0
		out.GroupPostsPerDay = 0
		out.ProfilePostsPerDay = 0
	}
	return out
}

func scaleCap(base int, mult float64) int {
	if base <= 0 {
		return base
	}
	v := float64(base) * mult
	if v < 0 {
		return 0
	}
	return int(v + 0.5)
}

// capsOverrideFields mirrors BehaviourCaps for JSON unmarshalling but uses
// pointers so absent fields are distinguishable from explicit zero. Only
// fields the operator sets are overlaid on the resolved caps.
type capsOverrideFields struct {
	CommentsPerDay       *int     `json:"comments_per_day"`
	InboxPerDay          *int     `json:"inbox_per_day"`
	GroupPostsPerDay     *int     `json:"group_posts_per_day"`
	ProfilePostsPerDay   *int     `json:"profile_posts_per_day"`
	SameGroupCooldownNs  *int64   `json:"same_group_cooldown_ns"`
	SamePostCooldownNs   *int64   `json:"same_post_cooldown_ns"`
	SameProfileCooldown  *int64   `json:"same_profile_cooldown_ns"`
	GlobalActionCooldown *int64   `json:"global_action_cooldown_ns"`
	RiskScoreCeiling     *float64 `json:"risk_score_ceiling"`
}

// OverlayCaps applies a per-account JSON override (from
// AccountBehaviourProfile.CapsOverride) on top of resolved caps. Empty
// or invalid JSON is ignored — the resolved caps pass through unchanged.
func OverlayCaps(base BehaviourCaps, overrideJSON string) BehaviourCaps {
	s := strings.TrimSpace(overrideJSON)
	if s == "" || s == "{}" {
		return base
	}
	var raw capsOverrideFields
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return base
	}
	out := base
	if raw.CommentsPerDay != nil {
		out.CommentsPerDay = *raw.CommentsPerDay
	}
	if raw.InboxPerDay != nil {
		out.InboxPerDay = *raw.InboxPerDay
	}
	if raw.GroupPostsPerDay != nil {
		out.GroupPostsPerDay = *raw.GroupPostsPerDay
	}
	if raw.ProfilePostsPerDay != nil {
		out.ProfilePostsPerDay = *raw.ProfilePostsPerDay
	}
	if raw.SameGroupCooldownNs != nil {
		out.SameGroupCooldown = time.Duration(*raw.SameGroupCooldownNs)
	}
	if raw.SamePostCooldownNs != nil {
		out.SamePostCooldown = time.Duration(*raw.SamePostCooldownNs)
	}
	if raw.SameProfileCooldown != nil {
		out.SameProfileCooldown = time.Duration(*raw.SameProfileCooldown)
	}
	if raw.GlobalActionCooldown != nil {
		out.GlobalActionCooldown = time.Duration(*raw.GlobalActionCooldown)
	}
	if raw.RiskScoreCeiling != nil {
		out.RiskScoreCeiling = *raw.RiskScoreCeiling
	}
	return out
}
