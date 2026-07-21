package models

import (
	"slices"
	"time"
)

// Multi-Actor Brand Coverage (spec: specs/domains/facebook-sales-intelligence/features/multi-actor-coverage/technical.md). A lead is
// SHARED; several accounts may comment it with different roles/tone/angle/CTA — this
// is brand coverage, NOT spam. CoveragePolicy caps how much / how often / how varied
// the org's touches on one lead may be; it does NOT default to one-comment-per-lead.

// CoveragePolicy is the per-org configuration. DefaultCoveragePolicy() is the
// brand-coverage-friendly baseline used until a settings surface exists.
type CoveragePolicy struct {
	AllowMultiActorCoverage     bool          `json:"allow_multi_actor_coverage"`
	MaxAccountsPerLead          int           `json:"max_accounts_per_lead"`
	MaxURLCommentsPerLead       int           `json:"max_url_comments_per_lead"`
	MaxDirectCTACommentsPerLead int           `json:"max_direct_cta_comments_per_lead"`
	MinGapBetweenActorComments  time.Duration `json:"min_gap_between_actor_comments"`
	RequireUniqueAngle          bool          `json:"require_unique_angle"`
	StopIfLeadReplies           bool          `json:"stop_if_lead_replies"`
}

func DefaultCoveragePolicy() CoveragePolicy {
	return CoveragePolicy{
		AllowMultiActorCoverage:     true,
		MaxAccountsPerLead:          2,
		MaxURLCommentsPerLead:       1,
		MaxDirectCTACommentsPerLead: 1,
		MinGapBetweenActorComments:  30 * time.Minute,
		RequireUniqueAngle:          true,
		StopIfLeadReplies:           true,
	}
}

// LeadCoverageState is the DERIVED, per-lead coverage picture — projected from the
// verified engagement ledger (and conversation thread), never from UI text.
type LeadCoverageState struct {
	ActorsTouched        []int64   `json:"actors_touched"`         // distinct account IDs with a verified touch
	OrgTouchCount        int       `json:"org_touch_count"`        // total verified touches on the lead
	WebsiteAlreadyUsed   bool      `json:"website_already_used"`   // a prior comment cited the website
	DirectCTAAlreadyUsed bool      `json:"direct_cta_already_used"`// a prior comment used a hard inbox CTA
	UsedAngles           []string  `json:"used_angles"`            // angle tags already taken
	LastTouchAt          time.Time `json:"last_touch_at"`          // zero if untouched
	LeadReplied          bool      `json:"lead_replied"`           // the lead replied to a touch
}

// ActorTouched reports whether the given account already has a verified touch.
func (s LeadCoverageState) ActorTouched(accountID int64) bool {
	return slices.Contains(s.ActorsTouched, accountID)
}

// Coverage eligibility reason codes (stable strings for the planner + UI mapping).
const (
	CoverageOK                    = "ok"
	CoverageAlreadyThisActor      = "already_commented_by_this_actor"
	CoverageLeadReplied           = "lead_replied"
	CoverageSingleActorPolicy     = "single_actor_policy"
	CoverageFull                  = "coverage_full"
	CoverageGapTooSoon            = "coverage_gap_too_soon"
)

// EvaluateCoverage decides whether actorAccountID may add a comment to a lead given
// its current coverage state + the org policy. Multi-actor coverage is ALLOWED by
// default. Pure (clock injected). Returns (eligible, reason).
func EvaluateCoverage(state LeadCoverageState, policy CoveragePolicy, actorAccountID int64, now time.Time) (bool, string) {
	// Execution is owned: one touch per actor — the same account never double-comments.
	if state.ActorTouched(actorAccountID) {
		return false, CoverageAlreadyThisActor
	}
	// Don't pile onto a lead that has engaged back.
	if policy.StopIfLeadReplies && state.LeadReplied {
		return false, CoverageLeadReplied
	}
	// Coverage disabled and someone else already touched it.
	if !policy.AllowMultiActorCoverage && state.OrgTouchCount > 0 {
		return false, CoverageSingleActorPolicy
	}
	// Enough accounts have covered this lead.
	if policy.MaxAccountsPerLead > 0 && state.OrgTouchCount >= policy.MaxAccountsPerLead {
		return false, CoverageFull
	}
	// Too close behind the previous actor's comment.
	if policy.MinGapBetweenActorComments > 0 && !state.LastTouchAt.IsZero() &&
		now.Sub(state.LastTouchAt) < policy.MinGapBetweenActorComments {
		return false, CoverageGapTooSoon
	}
	return true, CoverageOK
}

// ProjectLeadCoverage builds the coverage state from VERIFIED ledger entries + the
// lead-replied signal + the ACTUAL prior comment texts (spec:
// specs/domains/facebook-sales-intelligence/features/multi-actor-coverage/technical.md). Entries should already be verified
// successes; the check is applied defensively. Content fields are CONTENT-ACCURATE:
// website/CTA are true only if a comment actually used them, angles are classified
// from the text — never inferred from a touch count.
func ProjectLeadCoverage(entries []LeadEngagement, leadReplied bool, comments []string, website string) LeadCoverageState {
	st := LeadCoverageState{LeadReplied: leadReplied}
	for _, e := range entries {
		if !IsLedgerOutcomeVerifiedTouch(e.Outcome) {
			continue
		}
		if e.AccountID > 0 && !slices.Contains(st.ActorsTouched, e.AccountID) {
			st.ActorsTouched = append(st.ActorsTouched, e.AccountID)
		}
		st.OrgTouchCount++
		if e.PerformedAt.After(st.LastTouchAt) {
			st.LastTouchAt = e.PerformedAt
		}
	}
	st.WebsiteAlreadyUsed = DetectWebsiteUsed(comments, website)
	st.DirectCTAAlreadyUsed = DetectDirectCTAUsed(comments)
	st.UsedAngles = ClassifyAngles(comments)
	return st
}

// CTA + link policy values handed to the generator.
const (
	CTADirectInbox       = "direct_inbox"      // hard "inbox mình nhé" CTA
	CTAExperienceShare   = "experience_share"  // soft value-add, no hard CTA
	LinkMayIncludeWebsite = "may_include_website"
	LinkNoLink            = "no_link"
)

// ActorPersona shapes the next actor's comment so multi-actor coverage reads as a
// varied conversation, not a copy.
type ActorPersona struct {
	Role                     string   `json:"role"`
	Tone                     string   `json:"tone"`
	AllowedCTAStyle          string   `json:"allowed_cta_style"`
	ForbiddenRepeatedPhrases []string `json:"forbidden_repeated_phrases"`
	LinkPolicy               string   `json:"link_policy"`
}

// DeriveActorPersona reacts to what is already on the lead: if the website / hard CTA
// is taken, the next actor adds value (experience/advice) without repeating them, and
// must avoid the angles already used.
func DeriveActorPersona(state LeadCoverageState, policy CoveragePolicy, role, tone string) ActorPersona {
	p := ActorPersona{
		Role:                     role,
		Tone:                     tone,
		AllowedCTAStyle:          CTADirectInbox,
		LinkPolicy:               LinkMayIncludeWebsite,
		ForbiddenRepeatedPhrases: append([]string{}, state.UsedAngles...),
	}
	if state.WebsiteAlreadyUsed && policy.MaxURLCommentsPerLead <= 1 {
		p.LinkPolicy = LinkNoLink
	}
	if state.DirectCTAAlreadyUsed && policy.MaxDirectCTACommentsPerLead <= 1 {
		p.AllowedCTAStyle = CTAExperienceShare
	}
	return p
}
