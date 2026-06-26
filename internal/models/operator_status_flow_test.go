package models

import "testing"

// PR32A operator-visible status flow: characterize the "execution result ->
// operator-visible outcome" stage. VerifyOutcomeFromExecution is the single
// mapping that finalize / reconciler / reclassifiers share to decide what the
// operator sees on a finished outbound row; IsVerifiedSuccess is the gate behind
// the operator's "đã đăng" (verified-posted) state; IsTerminal decides whether a
// row is still actionable. Behavior-preserving: pins the current contract only.

// TestVerifyOutcomeFromExecution_FullMapping pins every ExecutionOutcome -> the
// VerificationOutcome the operator sees, plus the fail-closed default (an
// unrecognised outcome degrades to execution_failed with ok=false, never a silent
// success). Previously only the optimistic case was covered.
func TestVerifyOutcomeFromExecution_FullMapping(t *testing.T) {
	cases := []struct {
		in     ExecutionOutcome
		want   VerificationOutcome
		wantOK bool
	}{
		{ExecutionDOMVerified, VerifVerifiedSuccess, true},
		{ExecutionDuplicateBlocked, VerifVerifiedSuccess, true},
		{ExecutionOptimisticSuccess, VerifSubmittedUnverified, true}, // submitted != verified
		{ExecutionContextDrift, VerifContextDrift, true},
		{ExecutionRedirectedFeed, VerifContextDrift, true},
		{ExecutionTargetNotReached, VerifTargetNotReached, true},
		{ExecutionRateLimited, VerifRateLimited, true},
		{ExecutionBlocked, VerifBlocked, true},
		{ExecutionCaptcha, VerifCaptcha, true},
		{ExecutionShadowRejected, VerifShadowRejected, true},
		{ExecutionComposerFailed, VerifExecutionFailed, true},
		{ExecutionVerificationTimeout, VerifExecutionFailed, true},
		{ExecutionRetryExhausted, VerifExecutionFailed, true},
		{ExecutionHardFail, VerifExecutionFailed, true},
		{ExecutionSoftFail, VerifExecutionFailed, true},
		// Fail closed: an unknown/unmapped outcome must NOT surface as success.
		{ExecutionOutcome("totally_unknown"), VerifExecutionFailed, false},
		{ExecutionOutcome(""), VerifExecutionFailed, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.in), func(t *testing.T) {
			got, ok := VerifyOutcomeFromExecution(tc.in)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("VerifyOutcomeFromExecution(%q) = %q,%v; want %q,%v", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestIsVerifiedSuccess_OnlyFinishedVerified pins the engagement gate: ONLY
// (finished, verified_success) is a verified success. Submitted-unverified, any
// non-success outcome, and non-finished states must all read as NOT verified —
// so the operator never sees "đã đăng" for an unverified or in-flight row.
func TestIsVerifiedSuccess_OnlyFinishedVerified(t *testing.T) {
	if !IsVerifiedSuccess(ExecFinished, VerifVerifiedSuccess) {
		t.Fatal("(finished, verified_success) must be a verified success")
	}
	notVerified := []struct {
		state   ExecutionState
		outcome VerificationOutcome
	}{
		{ExecFinished, VerifSubmittedUnverified},
		{ExecFinished, VerifExecutionFailed},
		{ExecFinished, VerifContextDrift},
		{ExecExpired, VerifVerifiedSuccess}, // expired never observed a real success
		{ExecExecuting, VerifVerifiedSuccess},
		{ExecPlanned, ""},
	}
	for _, tc := range notVerified {
		if IsVerifiedSuccess(tc.state, tc.outcome) {
			t.Fatalf("IsVerifiedSuccess(%q,%q) must be false", tc.state, tc.outcome)
		}
	}
}

// TestIsTerminal pins which execution states are terminal (no longer actionable
// by claim/reset): finished and expired are terminal; planned and executing are not.
func TestIsTerminal(t *testing.T) {
	terminal := []ExecutionState{ExecFinished, ExecExpired}
	for _, s := range terminal {
		if !IsTerminal(s) {
			t.Fatalf("IsTerminal(%q) must be true", s)
		}
	}
	for _, s := range []ExecutionState{ExecPlanned, ExecExecuting} {
		if IsTerminal(s) {
			t.Fatalf("IsTerminal(%q) must be false", s)
		}
	}
}
