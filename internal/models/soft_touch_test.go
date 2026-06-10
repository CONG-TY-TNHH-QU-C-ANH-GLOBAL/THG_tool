package models

import "testing"

// Hard verified touch = only succeeded/dom_verified (test #5 + #1: submitted_unverified is NOT).
func TestIsLedgerOutcomeHardVerifiedTouch(t *testing.T) {
	hard := map[string]bool{
		"succeeded":            true,
		"dom_verified":         true,
		"submitted_unverified": false,
		"optimistic_success":   false,
		"failed":               false,
		"":                     false,
	}
	for outcome, want := range hard {
		if got := IsLedgerOutcomeHardVerifiedTouch(outcome); got != want {
			t.Errorf("IsLedgerOutcomeHardVerifiedTouch(%q) = %v, want %v", outcome, got, want)
		}
	}
}

// Soft touch = submitted_unverified/optimistic_success ONLY when the submit was accepted.
func TestIsLedgerOutcomeSoftTouch(t *testing.T) {
	if !IsLedgerOutcomeSoftTouch("submitted_unverified", true, true) {
		t.Error("submitted_unverified with submit+clear should be a soft touch")
	}
	if !IsLedgerOutcomeSoftTouch("optimistic_success", true, true) {
		t.Error("optimistic_success with submit+clear should be a soft touch")
	}
	if IsLedgerOutcomeSoftTouch("submitted_unverified", false, false) {
		t.Error("no submit → not a soft touch (must not block retry)")
	}
	if IsLedgerOutcomeSoftTouch("succeeded", true, true) {
		t.Error("succeeded is a HARD touch, not soft")
	}
	if IsLedgerOutcomeSoftTouch("failed", true, true) {
		t.Error("failed is never a soft touch")
	}
}

// Before-submit failures are retryable (the comment never landed) — test #2 support.
func TestIsRetryableBeforeSubmitFailure(t *testing.T) {
	retryable := []string{"target_not_reached", "composer_failed", "composer_clear_failed",
		"comment_text_doubled", "submit_button_not_found", "soft_fail"}
	for _, r := range retryable {
		if !IsRetryableBeforeSubmitFailure(r) {
			t.Errorf("%q should be a retryable before-submit failure", r)
		}
	}
	notRetryable := []string{"shadow_rejected", "blocked", "rate_limited", "captcha", "dom_verified"}
	for _, r := range notRetryable {
		if IsRetryableBeforeSubmitFailure(r) {
			t.Errorf("%q is post-submit/terminal, must NOT be retryable-before-submit", r)
		}
	}
}

// SubmitReachedForOutcome distinguishes post-submit from pre-submit outcomes.
func TestSubmitReachedForOutcome(t *testing.T) {
	reached := []ExecutionOutcome{ExecutionDOMVerified, ExecutionOptimisticSuccess, ExecutionShadowRejected, ExecutionBlocked}
	for _, o := range reached {
		if !SubmitReachedForOutcome(o) {
			t.Errorf("%s should imply submit reached", o)
		}
	}
	notReached := []ExecutionOutcome{ExecutionComposerFailed, ExecutionTargetNotReached, ExecutionContextDrift, ExecutionCaptcha}
	for _, o := range notReached {
		if SubmitReachedForOutcome(o) {
			t.Errorf("%s should imply submit NOT reached", o)
		}
	}
}
