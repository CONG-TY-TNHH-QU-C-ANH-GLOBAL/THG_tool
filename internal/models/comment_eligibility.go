package models

// Comment eligibility (§6 Dashboard alignment). A per-lead, read-only projection
// that explains — using the SAME gates as comment_all_leads — whether the lead can
// be commented now, by how many ready accounts, and why not when it can't. It
// carries only counts + reason codes + display copy: never tokens/cookies/secrets.
//
// It is computed in the SERVER layer (it needs connector readiness, which the
// leads store domain must not read) and rides along on LeadEngagementState; the
// store projection leaves it nil.

// Eligibility-state values beyond the coverage reason codes (CoverageFull,
// CoverageAlreadyThisActor, ...) reused from coverage.go.
const (
	EligibilityEligible      = "eligible"
	EligibilityNoReadyActor  = "no_ready_account" // accounts exist but none is connector-ready now
)

// CommentEligibility is the §6 contract. eligibility_state is "eligible" or a
// reason code; ineligibility_reason mirrors it when not eligible (else "").
type CommentEligibility struct {
	NextCommentEligible    bool   `json:"next_comment_eligible"`
	EligibilityState       string `json:"eligibility_state"`
	IneligibilityReason    string `json:"ineligibility_reason"`
	IneligibilityMessageVI string `json:"ineligibility_message_vi"`

	CommentedByCount int `json:"commented_by_count"` // distinct accounts with a verified touch
	MaxCoverage      int `json:"max_coverage"`       // CoveragePolicy.MaxAccountsPerLead

	CandidateActorCount int `json:"candidate_actor_count"` // Facebook accounts in the workspace
	ReadyActorCount     int `json:"ready_actor_count"`     // candidates with a ready connector now
	EligibleActorCount  int `json:"eligible_actor_count"`  // ready AND coverage-eligible for this lead

	LastCommentAttemptStatus string `json:"last_comment_attempt_status"` // queued|succeeded|failed|skipped|""
	LastCommentAttemptReason string `json:"last_comment_attempt_reason"` // typed reason when available
}
