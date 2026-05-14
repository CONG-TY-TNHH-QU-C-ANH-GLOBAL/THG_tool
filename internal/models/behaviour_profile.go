package models

import (
	"strings"
	"time"
)

// TrustLevel is the per-account POLICY PRESET that drives daily caps,
// cooldowns and gating decisions. It is intentionally an enum, not a
// numeric column — concrete caps live in the policy resolver, not on the
// account row. See feedback_behaviour_profile_design.md.
type TrustLevel string

const (
	TrustCold        TrustLevel = "cold"        // brand-new or unknown account, lowest caps
	TrustWarming     TrustLevel = "warming"     // conservative ramp, default for fresh-imported accounts
	TrustWarm        TrustLevel = "warm"        // established, moderate caps
	TrustTrusted     TrustLevel = "trusted"     // aggressive caps, few guards
	TrustSacrificial TrustLevel = "sacrificial" // burner / high-risk growth, treated as expendable
)

// NormalizeTrustLevel lowercases + trims an arbitrary string into a known
// TrustLevel. Unknown values fall back to TrustWarming — the safe default
// the system uses for accounts that have never been classified.
func NormalizeTrustLevel(s string) TrustLevel {
	switch TrustLevel(strings.ToLower(strings.TrimSpace(s))) {
	case TrustCold:
		return TrustCold
	case TrustWarming:
		return TrustWarming
	case TrustWarm:
		return TrustWarm
	case TrustTrusted:
		return TrustTrusted
	case TrustSacrificial:
		return TrustSacrificial
	default:
		return TrustWarming
	}
}

// WorkspaceRole is a free-form behavioural hint (e.g. "aggressive_outreach",
// "passive_engagement", "inbox_only") used together with TrustLevel by the
// policy resolver to pick caps. Free-form on purpose — operators define
// their own roles per workspace.
type WorkspaceRole string

// AccountBehaviourProfile is the STATIC identity of an account from the
// orchestrator's perspective. Low write rate. Updated by admin tooling,
// warmup workflows, or trust-promotion events.
type AccountBehaviourProfile struct {
	AccountID      int64         `json:"account_id"`
	OrgID          int64         `json:"org_id"`
	TrustLevel     TrustLevel    `json:"trust_level"`
	AccountAgeDays int           `json:"account_age_days"`
	PersonaType    string        `json:"persona_type"`
	WorkspaceRole  WorkspaceRole `json:"workspace_role"`
	Capabilities   string        `json:"capabilities"`   // raw JSON; resolved by callers
	CapsOverride   string        `json:"caps_override"`  // raw JSON; rare per-account override of resolver caps
	Notes          string        `json:"notes"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// AccountRuntimeState is the HIGH-CHURN runtime state of an account.
// Counters here roll over by date (CountersDay); the policy layer treats
// counter values as zero when CountersDay does not match "today".
//
// Contextual cooldowns (per group, per post, per profile) are NOT stored
// here — they are derived from ListActionLedger queries. Only the
// account-wide cooldown lives in CooldownUntil.
type AccountRuntimeState struct {
	AccountID         int64     `json:"account_id"`
	OrgID             int64     `json:"org_id"`
	CountersDay       string    `json:"counters_day"` // YYYY-MM-DD UTC
	CommentsToday     int       `json:"comments_today"`
	InboxToday        int       `json:"inbox_today"`
	GroupPostsToday   int       `json:"group_posts_today"`
	ProfilePostsToday int       `json:"profile_posts_today"`
	RiskScore         float64   `json:"risk_score"`
	RecentFailures    int       `json:"recent_failures"`
	CooldownUntil     time.Time `json:"cooldown_until"`
	LastActionAt      time.Time `json:"last_action_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CounterForAction returns the today-counter for an action type. Unknown
// action types return 0 — i.e. the policy layer treats them as unbounded
// by the per-action daily-cap mechanism (separate concerns enforce them).
func (r AccountRuntimeState) CounterForAction(action string) int {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "comment":
		return r.CommentsToday
	case "inbox":
		return r.InboxToday
	case "group_post":
		return r.GroupPostsToday
	case "profile_post":
		return r.ProfilePostsToday
	default:
		return 0
	}
}

// RiskSignal is the writer API for risk_score updates. The schema is
// multi-signal from day one so future signals (captcha frequency, redirect
// anomalies, action rejection rate, reply rate, browser crash, comment
// deletion) plug in without migration. v1 emits only Failure / Success.
type RiskSignal string

const (
	RiskSignalFailure         RiskSignal = "failure"
	RiskSignalSuccess         RiskSignal = "success"
	RiskSignalCaptcha         RiskSignal = "captcha"
	RiskSignalRedirectAnomaly RiskSignal = "redirect_anomaly"
	RiskSignalActionRejected  RiskSignal = "action_rejected"
	RiskSignalReplyReceived   RiskSignal = "reply_received"
	RiskSignalBrowserCrash    RiskSignal = "browser_crash"
	RiskSignalCommentDeleted  RiskSignal = "comment_deleted"
)

// SignalWeights is the default impact each signal type has on risk_score.
// Positive numbers raise risk (toward 1.0), negative numbers lower it.
// Callers may override per-signal weight; this is just the default table.
var SignalWeights = map[RiskSignal]float64{
	RiskSignalFailure:         0.05,
	RiskSignalSuccess:         -0.01,
	RiskSignalCaptcha:         0.15,
	RiskSignalRedirectAnomaly: 0.10,
	RiskSignalActionRejected:  0.08,
	RiskSignalReplyReceived:   -0.02,
	RiskSignalBrowserCrash:    0.04,
	RiskSignalCommentDeleted:  0.12,
}
