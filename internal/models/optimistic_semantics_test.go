package models

import "testing"

// Founder decision (Optimistic Success Semantics Hardening): optimistic_success is
// Submitted ≠ Verified — it must NOT be a verified touch anywhere.
func TestOptimisticSuccess_NotVerifiedTouch(t *testing.T) {
	// Ledger alias: optimistic → its own value, NOT "succeeded".
	if got := LedgerOutcomeAlias(ExecutionOptimisticSuccess); got != LedgerOutcomeSubmittedUnverified {
		t.Errorf("LedgerOutcomeAlias(optimistic) = %q, want %q", got, LedgerOutcomeSubmittedUnverified)
	}
	// Therefore it is NOT a verified ledger touch.
	if IsLedgerOutcomeVerifiedTouch(LedgerOutcomeAlias(ExecutionOptimisticSuccess)) {
		t.Error("optimistic_success must NOT be a verified ledger touch")
	}
	// Genuine verified outcomes still are.
	for _, o := range []ExecutionOutcome{ExecutionDOMVerified, ExecutionDuplicateBlocked} {
		if !IsLedgerOutcomeVerifiedTouch(LedgerOutcomeAlias(o)) {
			t.Errorf("%s should remain a verified ledger touch", o)
		}
	}
}

func TestOptimisticSuccess_Classifiers(t *testing.T) {
	if !IsSubmittedUnverifiedOutcome(ExecutionOptimisticSuccess) {
		t.Error("IsSubmittedUnverifiedOutcome(optimistic) should be true")
	}
	if IsVerifiedCommentOutcome(ExecutionOptimisticSuccess) {
		t.Error("IsVerifiedCommentOutcome(optimistic) must be false")
	}
	if !IsVerifiedCommentOutcome(ExecutionDOMVerified) || !IsVerifiedCommentOutcome(ExecutionDuplicateBlocked) {
		t.Error("dom_verified / duplicate_blocked should be verified comment outcomes")
	}
}

// The outbound row carries submitted_unverified (not verified_success), so the
// (state, outcome) success predicate is false → no engagement_events, UI shows
// "Đã gửi nhưng chưa xác minh".
func TestOptimisticSuccess_VerificationOutcome(t *testing.T) {
	out, ok := VerifyOutcomeFromExecution(ExecutionOptimisticSuccess)
	if !ok || out != VerifSubmittedUnverified {
		t.Errorf("VerifyOutcomeFromExecution(optimistic) = %q,%v; want submitted_unverified,true", out, ok)
	}
	state, outcome := TerminalFromOutcome(ExecutionOptimisticSuccess)
	if IsVerifiedSuccess(state, outcome) {
		t.Errorf("IsVerifiedSuccess(%s,%s) must be false for optimistic", state, outcome)
	}
	// dom_verified is still a verified success.
	vs, vo := TerminalFromOutcome(ExecutionDOMVerified)
	if !IsVerifiedSuccess(vs, vo) {
		t.Error("dom_verified must remain a verified success")
	}
}
