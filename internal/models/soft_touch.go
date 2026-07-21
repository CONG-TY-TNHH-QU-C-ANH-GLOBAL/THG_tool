package models

import "strings"

// Soft-touch semantics (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md, PR-1 Part B). A comment
// that was SUBMITTED but not DOM-verified (optimistic_success → ledger
// "submitted_unverified") is NOT a hard verified touch — it must never count as
// "succeeded" (feedback_verified_state_centric). But it IS a SOFT touch: the actor did
// click submit, so the planner/lifecycle must hold the lead in a verification cooldown
// (waiting_verification) instead of immediately re-commenting and risking a real duplicate
// on Facebook. These pure predicates centralise that distinction.

// IsLedgerOutcomeHardVerifiedTouch reports a GENUINELY verified touch — the only kind that
// mutates engagement/coverage state. Mirrors IsLedgerOutcomeVerifiedTouch but also accepts
// the rich "dom_verified" alias for callers reading execution_attempts directly.
func IsLedgerOutcomeHardVerifiedTouch(outcome string) bool {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case "succeeded", string(ExecutionDOMVerified):
		return true
	default:
		return false
	}
}

// IsLedgerOutcomeSoftTouch reports a SUBMITTED-but-unverified touch that should hold the
// lead in verification cooldown. True for the ledger value "submitted_unverified" or the
// rich "optimistic_success" — but ONLY when the submit was actually accepted
// (submitClicked AND composerCleared), so a pre-submit miss mislabelled upstream never
// blocks a legitimate retry.
func IsLedgerOutcomeSoftTouch(outcome string, submitClicked, composerCleared bool) bool {
	switch strings.ToLower(strings.TrimSpace(outcome)) {
	case LedgerOutcomeSubmittedUnverified, string(ExecutionOptimisticSuccess):
		return submitClicked && composerCleared
	default:
		return false
	}
}

// IsRetryableBeforeSubmitFailure reports a failure that happened BEFORE the comment was
// submitted — the action never landed on Facebook, so a fresh attempt is safe and the
// lead must NOT be treated as touched. Distinct from post-submit failures (shadow_rejected,
// blocked) where re-clicking risks a duplicate or digs the hole deeper.
func IsRetryableBeforeSubmitFailure(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "target_not_reached", "composer_failed", "composer_clear_failed",
		"comment_button_not_found", "comment_text_doubled", "comment_text_mismatch",
		"submit_button_not_found", "comment_submit_not_found", "submit_click_failed",
		"soft_fail", "verification_timeout":
		return true
	default:
		return false
	}
}

// SubmitReachedForOutcome derives, from a terminal ExecutionOutcome alone, whether the
// submit was actually fired (composer cleared). Used by the forensics report to fill
// submit_clicked / composer_cleared when the raw extension booleans were not persisted in
// evidence_json. Post-submit outcomes imply the click landed; pre-submit ones do not.
func SubmitReachedForOutcome(o ExecutionOutcome) bool {
	switch o {
	case ExecutionDOMVerified, ExecutionOptimisticSuccess, ExecutionDuplicateBlocked,
		ExecutionShadowRejected, ExecutionRedirectedFeed, ExecutionRateLimited,
		ExecutionBlocked:
		return true
	case ExecutionComposerFailed, ExecutionContextDrift, ExecutionTargetNotReached,
		ExecutionVerificationTimeout, ExecutionCaptcha, ExecutionSoftFail,
		ExecutionHardFail, ExecutionRetryExhausted:
		return false
	default:
		return false
	}
}
