package models

import "strings"

// Manual human verification + retry contracts (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md
// companion, Part A/B). Pure eligibility so the rules are single-sourced + unit-testable.

// Correction reasons + source written on the appended ledger / audit rows.
const (
	LedgerReasonHumanVerified = "human_verified"
	HumanVerifySource         = "operator_manual_confirm"
)

// HumanVerifyResult is what the endpoint returns after a manual confirm.
type HumanVerifyResult struct {
	Corrected           bool   `json:"corrected"`            // a new correction was appended
	AlreadyVerified     bool   `json:"already_verified"`     // idempotent: a correction already existed
	CorrectionLedgerID  int64  `json:"correction_ledger_id"` // the (new or existing) succeeded ledger row
	AuditID             int64  `json:"audit_id"`             // comment_verification_audit row id (0 if idempotent)
	NewEffectiveOutcome string `json:"new_effective_outcome"`
}

// CommentCorrection is a succeeded ledger correction on a comment (human_verified or
// reverified) — the signal the dashboard uses to show the LATEST EFFECTIVE outcome instead
// of the stale outbound_messages.verification_outcome.
type CommentCorrection struct {
	CorrectionID int64  `json:"correction_id"`
	Reason       string `json:"reason"`  // human_verified | reverified
	Outcome      string `json:"outcome"` // succeeded
}

// HumanVerifyEligible reports whether a comment may be MANUALLY confirmed as posted. Allowed
// ONLY for submitted_unverified — which by construction means the comment was SUBMITTED
// (click send) and the composer cleared (optimistic_success), we just couldn't machine-
// verify the node. NEVER allowed for failed_before_submit / target_not_reached /
// comment_button_not_found / composer_failed (nothing was submitted, so the operator must
// not be able to mark it posted). Returns (ok, reason).
func HumanVerifyEligible(msg OutboundMessage) (bool, string) {
	if msg.Type != "comment" {
		return false, "not_a_comment"
	}
	if msg.VerificationOutcome != VerifSubmittedUnverified {
		return false, "not_submitted_unverified" // covers every failed_before_submit case
	}
	if strings.TrimSpace(msg.TargetURL) == "" {
		return false, "no_target_url"
	}
	if strings.TrimSpace(msg.Content) == "" {
		return false, "no_expected_content"
	}
	if msg.AccountID <= 0 {
		return false, "no_account"
	}
	return true, "ok"
}

// CommentMetrics is the outcome summary (Part C) the admin/superadmin reads to decide
// whether submitted_unverified is frequent enough to reopen async reverify as a core bug.
type CommentMetrics struct {
	Total                 int `json:"total"`
	VerifiedSuccess       int `json:"verified_success"`
	SubmittedUnverified   int `json:"submitted_unverified"` // raw (before corrections)
	TargetNotReached      int `json:"target_not_reached"`
	ExecutionFailed       int `json:"execution_failed"` // includes comment_button_not_found
	CommentButtonNotFound int `json:"comment_button_not_found"`
	OtherFailed           int `json:"other_failed"`
	HumanVerified         int `json:"human_verified"` // corrections
	Reverified            int `json:"reverified"`     // corrections
	ReverifyError         int `json:"reverify_error"`
}

// EffectiveVerified counts comments verified by any means (machine + corrections).
func (m CommentMetrics) EffectiveVerified() int { return m.VerifiedSuccess + m.HumanVerified + m.Reverified }

// SubmittedUnverifiedOpen is the submitted_unverified still needing resolution.
func (m CommentMetrics) SubmittedUnverifiedOpen() int {
	open := m.SubmittedUnverified - m.HumanVerified - m.Reverified
	if open < 0 {
		return 0
	}
	return open
}

// SubmittedUnverifiedRate is the share of comments stuck submitted_unverified (0..1). The
// spec threshold: <0.10 = edge case (manual fallback); >0.10–0.15 sustained = reopen async
// reverify as a core reliability bug.
func (m CommentMetrics) SubmittedUnverifiedRate() float64 {
	if m.Total <= 0 {
		return 0
	}
	return float64(m.SubmittedUnverifiedOpen()) / float64(m.Total)
}

// IsRetryableVerificationOutcome reports whether a FINISHED comment outcome is a retryable
// pre-submit failure — the comment never landed, so a fresh attempt is safe. Covers
// target_not_reached and execution_failed (which is where comment_button_not_found /
// composer_failed collapse in the 2-column taxonomy). Post-submit failures (shadow_rejected,
// blocked, rate_limited) and submitted_unverified are NOT retryable here.
func IsRetryableVerificationOutcome(vo VerificationOutcome) bool {
	switch vo {
	case VerifTargetNotReached, VerifExecutionFailed:
		return true
	default:
		return false
	}
}
