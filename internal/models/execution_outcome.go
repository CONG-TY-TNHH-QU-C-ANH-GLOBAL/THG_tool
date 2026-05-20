package models

import (
	"strings"
	"time"
)

// ExecutionOutcome is the verified classification of a single outbound
// action attempt. REPLACES the binary queued|succeeded|failed|skipped
// taxonomy on action_ledger.outcome — those values remain valid as a
// back-compat alias but new writes use the values below.
//
// The taxonomy distinguishes failure modes so the Behaviour Profile risk
// writer ([[feedback-behaviour-profile-design]]) can emit a meaningful
// signal per outcome AND so the future orchestrator (PR-5) can learn
// account-specific failure patterns instead of treating every miss as
// equivalent. "click landed" is NOT in this list because it is not an
// outcome — only the post-DOM-state observation is.
type ExecutionOutcome string

const (
	// ExecutionDOMVerified — the platform accepted the action at a user-
	// visible DOM level. Comment node rendered AND count incremented AND
	// composer cleared (per-action recipe in internal/runtime/verifier.go).
	// The only "real success" outcome.
	ExecutionDOMVerified ExecutionOutcome = "dom_verified"

	// ExecutionOptimisticSuccess — click landed and no obvious failure
	// signal, but the DOM proof window expired without a positive match.
	// Treated as success for badge purposes; flagged for re-verification.
	// Use sparingly — silent shadow-rejects masquerade as this state.
	ExecutionOptimisticSuccess ExecutionOutcome = "optimistic_success"

	// ExecutionDuplicateBlocked — the verifier found the comment/message
	// was ALREADY present at attempt start. Treated as success for badge
	// purposes; do NOT re-submit. The classic idempotency case.
	ExecutionDuplicateBlocked ExecutionOutcome = "duplicate_blocked"

	// ExecutionShadowRejected — click landed, composer cleared, but the
	// expected DOM proof (comment node / message bubble) NEVER appeared
	// within the verification window. Facebook silently rejected. The
	// most dangerous failure mode — historically recorded as success.
	ExecutionShadowRejected ExecutionOutcome = "shadow_rejected"

	// ExecutionRateLimited — Facebook surfaced an explicit rate-limit
	// banner / toast ("You're posting too quickly"). Distinct from a
	// silent reject because the platform tells us.
	ExecutionRateLimited ExecutionOutcome = "rate_limited"

	// ExecutionComposerFailed — composer never opened, or opened but the
	// typed text didn't land, or submit button never became clickable.
	// Pre-submit failure; the action never actually fired.
	ExecutionComposerFailed ExecutionOutcome = "composer_failed"

	// ExecutionContextDrift — browser landed somewhere unexpected during
	// the action (newsfeed instead of the target post, profile redirect).
	// Same family as ErrFacebookContextDrift but observed on the EXECUTE
	// side rather than the CRAWL side.
	ExecutionContextDrift ExecutionOutcome = "context_drift"

	// ExecutionRedirectedFeed — page navigated to home.php / newsfeed
	// after submit. Specific case of context_drift seen as a comment
	// failure mode (FB redirects on auth/throttle).
	ExecutionRedirectedFeed ExecutionOutcome = "redirected_feed"

	// ExecutionVerificationTimeout — DOM verifier ran but couldn't read
	// the page (chromedp error, eval failure). Distinct from shadow-reject
	// because we don't actually know the platform's decision.
	ExecutionVerificationTimeout ExecutionOutcome = "verification_timeout"

	// ExecutionCaptcha — challenge page intercepted the flow. Account
	// is hard-paused; risk signal escalates; human handoff queued (future).
	ExecutionCaptcha ExecutionOutcome = "captcha"

	// ExecutionBlocked — Facebook actively rejected the action (post
	// deleted by author, account muted on group, banner about violation).
	// Deterministic failure — distinct from shadow_rejected because the
	// platform told us.
	ExecutionBlocked ExecutionOutcome = "blocked"

	// ExecutionRetryExhausted — retried up to the policy limit and never
	// reached a verified outcome. Terminal failure for this attempt chain.
	ExecutionRetryExhausted ExecutionOutcome = "retry_exhausted"

	// ExecutionHardFail — deterministic executor error (selector missing,
	// JS exception in verifier, browser crash mid-action). Not retryable.
	ExecutionHardFail ExecutionOutcome = "hard_fail"

	// ExecutionSoftFail — transient (network blip, page load timeout).
	// Retry-safe. Distinct from shadow_rejected because we didn't even
	// reach the submit.
	ExecutionSoftFail ExecutionOutcome = "soft_fail"
)

// AttemptStatus is the lifecycle marker for a single execution_attempts
// row. Distinct from ExecutionOutcome: status is "where are we in the
// click pipeline RIGHT NOW", outcome is "what classification did the
// post-DOM observation reach". A row transitions through statuses and
// terminates with an outcome.
type AttemptStatus string

const (
	AttemptQueued          AttemptStatus = "queued"
	AttemptComposerOpened  AttemptStatus = "composer_opened"
	AttemptTyped           AttemptStatus = "typed"
	AttemptSubmitted       AttemptStatus = "submitted"
	AttemptVerifying       AttemptStatus = "verifying"
	AttemptDOMVerified     AttemptStatus = "dom_verified"
	AttemptFailed          AttemptStatus = "failed"
)

// IsSuccessOutcome reports whether the outcome should be treated as
// "real engagement" by the LeadEngagement projection and badge derivation.
// Only the three success-class outcomes count; everything else is a
// non-event for coordination purposes (do NOT promote a lead to
// `protected` based on a shadow_rejected attempt).
func IsSuccessOutcome(o ExecutionOutcome) bool {
	switch o {
	case ExecutionDOMVerified, ExecutionOptimisticSuccess, ExecutionDuplicateBlocked:
		return true
	default:
		return false
	}
}

// IsRetryableOutcome reports whether a fresh attempt is likely to
// succeed. soft_fail / verification_timeout are retryable; shadow_reject
// / blocked / captcha / rate_limited are NOT — re-clicking would just
// dig the hole deeper.
func IsRetryableOutcome(o ExecutionOutcome) bool {
	switch o {
	case ExecutionSoftFail, ExecutionVerificationTimeout, ExecutionComposerFailed:
		return true
	default:
		return false
	}
}

// RiskSignalForOutcome maps a verified outcome onto the behaviour-profile
// risk signal that should fire. The orchestrator (PR-5) reads risk_score
// downstream of this mapping — it is the bridge between "the executor saw
// X" and "the account's reputation moved Y". Empty string = no signal.
func RiskSignalForOutcome(o ExecutionOutcome) RiskSignal {
	switch o {
	case ExecutionDOMVerified, ExecutionDuplicateBlocked:
		return RiskSignalSuccess
	case ExecutionOptimisticSuccess:
		return "" // ambiguous — don't move risk in either direction
	case ExecutionShadowRejected:
		return RiskSignalActionRejected
	case ExecutionRateLimited:
		return RiskSignalActionRejected
	case ExecutionRedirectedFeed, ExecutionContextDrift:
		return RiskSignalRedirectAnomaly
	case ExecutionCaptcha:
		return RiskSignalCaptcha
	case ExecutionBlocked:
		return RiskSignalActionRejected
	case ExecutionVerificationTimeout, ExecutionSoftFail:
		return "" // transient — don't poison the profile
	case ExecutionComposerFailed, ExecutionHardFail, ExecutionRetryExhausted:
		return RiskSignalFailure
	default:
		return ""
	}
}

// LedgerOutcomeAlias maps the rich ExecutionOutcome onto the legacy
// action_ledger.outcome string column so MarkActionLedgerOutcome stays
// compatible with the existing 4-value taxonomy until callers migrate.
// Verified successes collapse to "succeeded"; everything failure-class
// collapses to "failed" (with the rich classification in the reason
// column). The execution_attempts table holds the full taxonomy.
func LedgerOutcomeAlias(o ExecutionOutcome) string {
	if IsSuccessOutcome(o) {
		return "succeeded"
	}
	return "failed"
}

// Activity feed event names (project goal, May-2026). The legacy
// system_outbound_queued / system_outbound_status / etc. events lump
// every state transition into a single bucket. The autonomous-verified
// model splits them into four distinct events so the operator-replay
// UI and the AI planner can read them apart:
//
//   ExecutionEventPlanned   — outbound row inserted in planned state
//                             (was: "queued" / "drafted" / "approved").
//   ExecutionEventStarted   — extension claimed the row and began the
//                             execute path (was: status flip to
//                             "sending").
//   ExecutionEventVerified  — DOM verifier confirmed the action
//                             actually landed at the intended target.
//                             This is the ONLY event that promotes a
//                             lead to "touched".
//   ExecutionEventFailed    — any non-verified terminal (verified_failure,
//                             context_drift, blocked, rate_limited,
//                             expired). The specific reason is in the
//                             event payload's failure_reason field.
const (
	ExecutionEventPlanned  = "execution_planned"
	ExecutionEventStarted  = "execution_started"
	ExecutionEventVerified = "execution_verified"
	ExecutionEventFailed   = "execution_failed"
)

// TerminalFromOutcome maps a rich ExecutionOutcome onto the
// (ExecutionState, VerificationOutcome) pair the finalize path should
// land. PR-1 split: this is the single bridge between the rich
// execution_attempts taxonomy and the 2-column outbound_messages
// taxonomy. The store-layer finalize path calls into this to write
// both columns atomically.
//
// ExecExpired is returned ONLY for ExecutionRetryExhausted — that
// outcome explicitly means "we gave up retrying, never reached a
// verified DOM observation". Every other terminal outcome carries
// some kind of observation, so ExecFinished is the correct state
// and the verification_outcome column captures the detail.
func TerminalFromOutcome(o ExecutionOutcome) (ExecutionState, VerificationOutcome) {
	if o == ExecutionRetryExhausted {
		return ExecExpired, ""
	}
	outcome, _ := VerifyOutcomeFromExecution(o)
	return ExecFinished, outcome
}

// IsLedgerOutcomeVerifiedTouch is the single source of truth for
// "does this action_ledger row count as a verified customer touch?".
//
// The autonomous-verified-execution model (see project goal,
// May-2026) mandates that lead engagement state mutates ONLY after a
// DOM-verified success. Anything else — queued, failed,
// context_drift, blocked, rate_limited, skipped — is NOT a touch;
// the customer was never contacted by us.
//
// Callers that surface "Đã chạm" / engagement badges, dedupe by
// touched-lead, or feed verified-only state into the AI planner
// MUST gate on this predicate. The Lead Engagement projection in
// internal/store/lead_engagement.go filters its SQL on this; the
// DeriveBadge function in lead_engagement.go also re-filters as
// defense in depth.
//
// Returns true ONLY for "succeeded". Note that the rich
// ExecutionOutcome taxonomy collapses through LedgerOutcomeAlias —
// dom_verified / optimistic_success / duplicate_blocked all map to
// "succeeded" here, which is the right behaviour: duplicate_blocked
// means the comment already existed, optimistic_success has partial
// proof, and dom_verified is the strongest signal — all three
// represent a real touch from the customer's point of view.
func IsLedgerOutcomeVerifiedTouch(outcome string) bool {
	return outcome == "succeeded"
}

// ExecutionAttempt mirrors one row of execution_attempts. The store layer
// constructs/serialises these; verifier callers fill out the Outcome +
// Evidence fields after the post-submit observation.
type ExecutionAttempt struct {
	ID             int64            `json:"id"`
	ActionLedgerID int64            `json:"action_ledger_id"`
	OutboundID     int64            `json:"outbound_id"`
	OrgID          int64            `json:"org_id"`
	AccountID      int64            `json:"account_id"`
	TargetURL      string           `json:"target_url"`
	ActionType     string           `json:"action_type"`
	Attempt        int              `json:"attempt"`
	Status         AttemptStatus    `json:"status"`
	Outcome        ExecutionOutcome `json:"outcome"`
	FailureReason  string           `json:"failure_reason"`
	EvidenceJSON   string           `json:"evidence_json"`
	StartedAt      time.Time        `json:"started_at"`
	FinishedAt     time.Time        `json:"finished_at"`
	DOMVerified    bool             `json:"dom_verified"`
	NetworkVerified bool            `json:"network_verified"`
}

// NormalizeExecutionOutcome maps free-form input onto a known outcome.
// Unknown strings fall back to ExecutionHardFail — the safe default that
// flags the row as failure without faking success.
func NormalizeExecutionOutcome(s string) ExecutionOutcome {
	switch ExecutionOutcome(strings.ToLower(strings.TrimSpace(s))) {
	case ExecutionDOMVerified:
		return ExecutionDOMVerified
	case ExecutionOptimisticSuccess:
		return ExecutionOptimisticSuccess
	case ExecutionDuplicateBlocked:
		return ExecutionDuplicateBlocked
	case ExecutionShadowRejected:
		return ExecutionShadowRejected
	case ExecutionRateLimited:
		return ExecutionRateLimited
	case ExecutionComposerFailed:
		return ExecutionComposerFailed
	case ExecutionContextDrift:
		return ExecutionContextDrift
	case ExecutionRedirectedFeed:
		return ExecutionRedirectedFeed
	case ExecutionVerificationTimeout:
		return ExecutionVerificationTimeout
	case ExecutionCaptcha:
		return ExecutionCaptcha
	case ExecutionBlocked:
		return ExecutionBlocked
	case ExecutionRetryExhausted:
		return ExecutionRetryExhausted
	case ExecutionSoftFail:
		return ExecutionSoftFail
	default:
		return ExecutionHardFail
	}
}
