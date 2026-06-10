package models

// Comment Verification Forensics (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-1 Part A). A
// read-only diagnostic shape: per (outbound, attempt, ledger) it surfaces the persisted
// evidence and a classification that separates "failed before submit" from "submitted but
// verify could not confirm" from "really failed". Submit/verify booleans are DERIVED from
// the terminal ExecutionOutcome where the raw extension booleans were not persisted in
// evidence_json (see SubmitReachedForOutcome).

// CommentForensicsClass buckets a comment attempt for triage.
const (
	ForensicsFailedBeforeSubmit   = "failed_before_submit"          // never reached submit; safe retry
	ForensicsSubmittedUnverified  = "submitted_unverified"          // optimistic_success; soft touch
	ForensicsLikelyVerifyFalseNeg = "likely_verify_false_negative"  // submit landed, verify missed it → reverify
	ForensicsRealFailed           = "real_failed"                   // platform explicitly rejected
	ForensicsRedirectedDrift      = "redirected_or_context_drift"   // navigated away / wrong post
	ForensicsVerified             = "verified"                      // hard DOM-verified success
	ForensicsUnknown              = "unknown"
)

// CommentForensicsRow is one comment attempt's full forensic record.
type CommentForensicsRow struct {
	OutboundID                 int64  `json:"outbound_id"`
	ExecutionID                string `json:"execution_id"`
	TargetURL                  string `json:"target_url"`
	AccountID                  int64  `json:"account_id"`
	ActorDisplay               string `json:"actor_display"`
	ExecutionState             string `json:"execution_state"`
	VerificationOutcome        string `json:"verification_outcome"`
	LedgerOutcome              string `json:"ledger_outcome"`
	AttemptOutcome             string `json:"attempt_outcome"`
	FailureReason              string `json:"failure_reason"`
	SubmitReached              bool   `json:"submit_reached"`               // derived from outcome
	ComposerClearedAfterSubmit bool   `json:"composer_cleared_after_submit"` // derived from outcome
	VerifierFoundComment       bool   `json:"verifier_found_comment"`        // derived: dom_verified
	CommentPermalink           string `json:"comment_permalink"`
	PageURLAfter               string `json:"page_url_after"`
	RedirectClass              string `json:"redirect_class"`
	Phase                      string `json:"phase"`
	NavDiagnosticSummary       string `json:"nav_diagnostic_summary"`
	EvidenceScreenshotPath     string `json:"evidence_screenshot_path"`
	Notes                      string `json:"notes"`
	Classification             string `json:"classification"`

	// Async-reverify observability (PR-A.1). These answer "did the reverify pipeline run,
	// and did it correct this comment?" — so a stuck submitted_unverified can be traced to
	// not-scheduled vs scheduled-not-claimed vs attempted-not-found vs corrected.
	ReverifyScheduled      bool   `json:"reverify_scheduled"`       // a comment_reverify row exists
	ReverifyOutcome        string `json:"reverify_outcome"`         // pending | verified | not_found | error | ""
	ReverifyAttemptedAt    string `json:"reverify_attempted_at"`    // when the connector reported (or "")
	ReverifyReason         string `json:"reverify_reason"`          // reverify diagnostic reason
	CorrectionEventID      int64  `json:"correction_event_id"`      // action_ledger.id of the appended 'succeeded' correction (0 = none)
	LatestEffectiveOutcome string `json:"latest_effective_outcome"` // succeeded if a correction exists, else the latest ledger outcome
}

// ClassifyCommentForensics buckets an attempt from its terminal outcome. The outcome
// already encodes whether submit was reached; the rich permalink/node booleans were not
// persisted, so shadow_rejected WITH a reached submit is flagged as a likely verify
// false-negative (the comment probably posted but the in-window check missed it).
func ClassifyCommentForensics(attemptOutcome string) string {
	o := NormalizeExecutionOutcome(attemptOutcome)
	switch o {
	case ExecutionDOMVerified, ExecutionDuplicateBlocked:
		return ForensicsVerified
	case ExecutionOptimisticSuccess:
		return ForensicsSubmittedUnverified
	case ExecutionRedirectedFeed, ExecutionContextDrift:
		return ForensicsRedirectedDrift
	case ExecutionBlocked, ExecutionRateLimited, ExecutionCaptcha:
		return ForensicsRealFailed
	case ExecutionShadowRejected:
		if SubmitReachedForOutcome(o) {
			return ForensicsLikelyVerifyFalseNeg
		}
		return ForensicsRealFailed
	case ExecutionComposerFailed, ExecutionTargetNotReached, ExecutionVerificationTimeout, ExecutionSoftFail:
		return ForensicsFailedBeforeSubmit
	case ExecutionHardFail, ExecutionRetryExhausted:
		if !SubmitReachedForOutcome(o) {
			return ForensicsFailedBeforeSubmit
		}
		return ForensicsRealFailed
	default:
		return ForensicsUnknown
	}
}

// FillDerivedForensics populates the derived submit/verify booleans + classification from
// the attempt outcome. Call after the store sets the persisted fields.
func (r *CommentForensicsRow) FillDerivedForensics() {
	o := NormalizeExecutionOutcome(r.AttemptOutcome)
	r.SubmitReached = SubmitReachedForOutcome(o)
	r.ComposerClearedAfterSubmit = r.SubmitReached
	r.VerifierFoundComment = IsVerifiedCommentOutcome(o)
	r.Classification = ClassifyCommentForensics(r.AttemptOutcome)
}
