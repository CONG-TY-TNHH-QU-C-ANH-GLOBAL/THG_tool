package models

import "testing"

// Success-class outcomes are the ONLY ones that should promote a lead's
// badge to `protected` / `followup_pending`. Anything else (shadow,
// blocked, captcha, rate_limited, …) must NOT count as engagement —
// that is the hallucination bug Step 3 closes.
func TestIsSuccessOutcome(t *testing.T) {
	success := []ExecutionOutcome{
		ExecutionDOMVerified,
		ExecutionOptimisticSuccess,
		ExecutionDuplicateBlocked,
	}
	for _, o := range success {
		if !IsSuccessOutcome(o) {
			t.Errorf("IsSuccessOutcome(%q) = false; want true", o)
		}
	}
	failures := []ExecutionOutcome{
		ExecutionShadowRejected,
		ExecutionRateLimited,
		ExecutionComposerFailed,
		ExecutionContextDrift,
		ExecutionRedirectedFeed,
		ExecutionVerificationTimeout,
		ExecutionCaptcha,
		ExecutionBlocked,
		ExecutionRetryExhausted,
		ExecutionHardFail,
		ExecutionSoftFail,
	}
	for _, o := range failures {
		if IsSuccessOutcome(o) {
			t.Errorf("IsSuccessOutcome(%q) = true; want false", o)
		}
	}
}

// Retry policy invariant: only transient failures retry. shadow_rejected,
// captcha, blocked, rate_limited MUST NOT retry — re-clicking digs the
// hole deeper (more rate-limit triggers, more captcha challenges).
func TestIsRetryableOutcome(t *testing.T) {
	retryable := []ExecutionOutcome{
		ExecutionSoftFail,
		ExecutionVerificationTimeout,
		ExecutionComposerFailed,
	}
	for _, o := range retryable {
		if !IsRetryableOutcome(o) {
			t.Errorf("IsRetryableOutcome(%q) = false; want true", o)
		}
	}
	notRetryable := []ExecutionOutcome{
		ExecutionDOMVerified,
		ExecutionShadowRejected,
		ExecutionRateLimited,
		ExecutionCaptcha,
		ExecutionBlocked,
		ExecutionDuplicateBlocked,
		ExecutionRedirectedFeed,
	}
	for _, o := range notRetryable {
		if IsRetryableOutcome(o) {
			t.Errorf("IsRetryableOutcome(%q) = true; want false", o)
		}
	}
}

// RiskSignalForOutcome is the bridge from execution outcome to behaviour
// profile mutation. The mapping is load-bearing — wrong signal → wrong
// risk_score movement → wrong orchestrator decision.
func TestRiskSignalForOutcome(t *testing.T) {
	cases := []struct {
		outcome ExecutionOutcome
		want    RiskSignal
	}{
		{ExecutionDOMVerified, RiskSignalSuccess},
		{ExecutionDuplicateBlocked, RiskSignalSuccess},
		{ExecutionShadowRejected, RiskSignalActionRejected},
		{ExecutionRateLimited, RiskSignalActionRejected},
		{ExecutionRedirectedFeed, RiskSignalRedirectAnomaly},
		{ExecutionContextDrift, RiskSignalRedirectAnomaly},
		{ExecutionCaptcha, RiskSignalCaptcha},
		{ExecutionBlocked, RiskSignalActionRejected},
		{ExecutionComposerFailed, RiskSignalFailure},
		{ExecutionHardFail, RiskSignalFailure},
		{ExecutionRetryExhausted, RiskSignalFailure},
		// Ambiguous outcomes deliberately emit no signal — do NOT move risk
		// in either direction when we don't know what actually happened.
		{ExecutionOptimisticSuccess, ""},
		{ExecutionSoftFail, ""},
		{ExecutionVerificationTimeout, ""},
	}
	for _, c := range cases {
		got := RiskSignalForOutcome(c.outcome)
		if got != c.want {
			t.Errorf("RiskSignalForOutcome(%q) = %q; want %q", c.outcome, got, c.want)
		}
	}
}

// LedgerOutcomeAlias collapses the rich taxonomy onto the legacy 4-value
// outcome column. Verified-success-family → "succeeded"; everything else
// → "failed". Detail rides in the reason field, not this collapse.
func TestLedgerOutcomeAlias(t *testing.T) {
	if got := LedgerOutcomeAlias(ExecutionDOMVerified); got != "succeeded" {
		t.Errorf("dom_verified → ledger = %q; want succeeded", got)
	}
	if got := LedgerOutcomeAlias(ExecutionShadowRejected); got != "failed" {
		t.Errorf("shadow_rejected → ledger = %q; want failed", got)
	}
	if got := LedgerOutcomeAlias(ExecutionCaptcha); got != "failed" {
		t.Errorf("captcha → ledger = %q; want failed", got)
	}
}

// Unknown strings default to hard_fail — the safe choice. Defaulting to
// "success" would re-introduce the hallucination bug; defaulting to a
// retryable status would loop forever.
func TestNormalizeExecutionOutcome_UnknownFallsBackToHardFail(t *testing.T) {
	if got := NormalizeExecutionOutcome("totally_made_up"); got != ExecutionHardFail {
		t.Errorf("unknown → %q; want hard_fail", got)
	}
	if got := NormalizeExecutionOutcome(""); got != ExecutionHardFail {
		t.Errorf("empty → %q; want hard_fail", got)
	}
	// Casing + whitespace tolerated.
	if got := NormalizeExecutionOutcome("  DOM_VERIFIED  "); got != ExecutionDOMVerified {
		t.Errorf("case+space normalised → %q; want dom_verified", got)
	}
}
