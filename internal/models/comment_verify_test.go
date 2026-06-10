package models

import "testing"

// Manual confirm is allowed ONLY for submitted_unverified (with submit evidence) — never for
// any failed_before_submit variant.
func TestHumanVerifyEligible(t *testing.T) {
	base := OutboundMessage{Type: "comment", TargetURL: "https://fb.com/p/1", Content: "hi", AccountID: 10}

	ok := base
	ok.VerificationOutcome = VerifSubmittedUnverified
	if eligible, _ := HumanVerifyEligible(ok); !eligible {
		t.Error("submitted_unverified with url+content+account should be eligible")
	}

	// Every failed_before_submit variant must be rejected.
	for _, vo := range []VerificationOutcome{VerifTargetNotReached, VerifExecutionFailed, VerifVerifiedSuccess, ""} {
		bad := base
		bad.VerificationOutcome = vo
		if eligible, reason := HumanVerifyEligible(bad); eligible {
			t.Errorf("verification_outcome=%q must NOT be manually confirmable (reason=%q)", vo, reason)
		}
	}

	// Missing evidence fields are rejected even when submitted_unverified.
	for _, mut := range []func(m *OutboundMessage){
		func(m *OutboundMessage) { m.TargetURL = "" },
		func(m *OutboundMessage) { m.Content = "" },
		func(m *OutboundMessage) { m.AccountID = 0 },
		func(m *OutboundMessage) { m.Type = "inbox" },
	} {
		bad := base
		bad.VerificationOutcome = VerifSubmittedUnverified
		mut(&bad)
		if eligible, _ := HumanVerifyEligible(bad); eligible {
			t.Errorf("missing-evidence record must be rejected: %+v", bad)
		}
	}
}

// Retry is offered for pre-submit failures only.
func TestIsRetryableVerificationOutcome(t *testing.T) {
	retry := []VerificationOutcome{VerifTargetNotReached, VerifExecutionFailed}
	for _, vo := range retry {
		if !IsRetryableVerificationOutcome(vo) {
			t.Errorf("%q should be retryable", vo)
		}
	}
	for _, vo := range []VerificationOutcome{VerifSubmittedUnverified, VerifVerifiedSuccess} {
		if IsRetryableVerificationOutcome(vo) {
			t.Errorf("%q must NOT be retryable", vo)
		}
	}
}

func TestCommentMetrics_Derived(t *testing.T) {
	m := CommentMetrics{Total: 100, VerifiedSuccess: 70, SubmittedUnverified: 10, HumanVerified: 4, Reverified: 1}
	if m.EffectiveVerified() != 75 {
		t.Errorf("effective verified = %d, want 75", m.EffectiveVerified())
	}
	if m.SubmittedUnverifiedOpen() != 5 {
		t.Errorf("open = %d, want 5", m.SubmittedUnverifiedOpen())
	}
	if r := m.SubmittedUnverifiedRate(); r < 0.049 || r > 0.051 {
		t.Errorf("rate = %f, want ~0.05", r)
	}
}
