package models

// Outbound state model — two orthogonal dimensions.
//
// Why two columns instead of one: the legacy single `status` column trộn
// transport lifecycle (planned/executing/finished/expired) with
// verification result (verified_success/context_drift/rate_limited/etc).
// Query "bao nhiêu attempt drift trong tuần" buộc phải filter
// `status IN (...)` + cross-check execution_attempts. Fragile.
//
// Splitting into two dimensions lets analytics aggregate independently:
//   - "execution funnel": planned → executing → finished/expired
//   - "verification mix": of finished rows, how many verified vs drift
//
// Hard invariant: engagement_events row is emitted (PR-2) ONLY when
// (execution_state='finished' AND verification_outcome='verified_success'
// AND EnforceTargetIdentity passed AND execution_id CAS matched).

// ExecutionState is the transport lifecycle of an outbound row.
// Mutable state machine — outbound transitions through these on its
// path from queue to terminal. KHÔNG carry verification semantics.
type ExecutionState string

const (
	// ExecPlanned — row inserted, waiting for an executor to claim.
	// No execution_id stamped yet. Equivalent to the legacy "approved"
	// state (autonomous-first model has no human-approval gate so
	// "draft" no longer exists).
	ExecPlanned ExecutionState = "planned"

	// ExecExecuting — an executor (Chrome Extension) claimed the row,
	// execution_id stamped, lease_expiry armed. Action is in flight.
	// Equivalent to the legacy "sending" state.
	ExecExecuting ExecutionState = "executing"

	// ExecFinished — the executor returned a terminal report and the
	// CAS finalize landed. The row's verification_outcome holds the
	// classification. Reading "did this action succeed?" requires
	// inspecting verification_outcome — finished does NOT imply success.
	ExecFinished ExecutionState = "finished"

	// ExecExpired — the row's lease_expiry passed before any executor
	// finalized it. The action never reached a verified outcome and
	// the system cannot retroactively determine whether it landed on
	// the platform. verification_outcome is NULL by definition.
	ExecExpired ExecutionState = "expired"
)

// VerificationOutcome is the post-DOM-observation classification of a
// finished execution. NULL for planned/executing/expired rows (no
// observation made yet, or never made). Each value represents a
// distinct platform/runtime signal — collapsing them onto a single
// "failed" bucket loses the information needed to:
//   - emit / withhold engagement_events (only verified_success emits)
//   - downgrade account reputation (shadow_rejected / blocked / captcha
//     mean very different things)
//   - choose retry strategy (rate_limited is wait-and-retry, blocked
//     is hard-stop, context_drift means the DOM target was wrong)
//   - surface meaningful KPIs to the operator
type VerificationOutcome string

const (
	// VerifVerifiedSuccess — DOM-verified the action landed on the
	// intended target AND target identity invariant held AND the
	// execution_id CAS matched. The only outcome that emits an
	// engagement_events row (PR-2).
	VerifVerifiedSuccess VerificationOutcome = "verified_success"

	// VerifContextDrift — the action landed on a different post /
	// thread / entity than the outbound row's target_url. Identity
	// invariant violated. Engagement event WITHHELD. Account reputation
	// may downgrade depending on frequency.
	VerifContextDrift VerificationOutcome = "context_drift"

	// VerifRateLimited — platform surfaced an explicit rate-limit
	// signal (banner, 429, "posting too quickly"). Action did NOT land.
	// Account reputation downgrades; retry deferred with backoff.
	VerifRateLimited VerificationOutcome = "rate_limited"

	// VerifBlocked — platform actively rejected (post deleted, account
	// muted on the surface, violation banner). Deterministic failure.
	// Account reputation downgrades; do NOT retry.
	VerifBlocked VerificationOutcome = "blocked"

	// VerifCaptcha — challenge / login wall intercepted the executor.
	// Human required. Account is hard-paused; future outbound for this
	// account refuses until manual intervention.
	VerifCaptcha VerificationOutcome = "captcha"

	// VerifShadowRejected — click landed, composer cleared, but the
	// expected DOM proof never appeared within the verification window.
	// Platform silently filtered. Historically the most dangerous
	// failure mode — was being recorded as success before PR-1.
	VerifShadowRejected VerificationOutcome = "shadow_rejected"

	// VerifExecutionFailed — deterministic executor / runtime failure
	// (selector missing, JS exception, browser crash mid-action). Not
	// a platform signal — a system fault.
	VerifExecutionFailed VerificationOutcome = "execution_failed"

	// VerifTargetNotReached — the navigation never put the queued post on
	// the page (PR8A landing gate). The executor stopped before typing.
	// Distinct from context_drift: no article was ever reached, so there
	// was nothing to drift FROM. Surfaces to the operator as
	// `finished/target_not_reached`; the attempt's NavDiagnostic.RedirectClass
	// names the precise cause. Engagement event WITHHELD; account reputation
	// is NOT downgraded (a navigation problem, not an account problem).
	VerifTargetNotReached VerificationOutcome = "target_not_reached"
)

// VerifyOutcomeFromExecution maps the rich ExecutionOutcome (from
// internal/models/execution_outcome.go — used by execution_attempts
// rows) onto the VerificationOutcome on the outbound row. Centralised
// so finalize, reconciler, and any future bulk reclassifier agree.
//
// Returns (outcome, ok). ok=false means the input couldn't be mapped —
// callers should treat as VerifExecutionFailed defensively.
func VerifyOutcomeFromExecution(o ExecutionOutcome) (VerificationOutcome, bool) {
	switch o {
	case ExecutionDOMVerified, ExecutionOptimisticSuccess, ExecutionDuplicateBlocked:
		return VerifVerifiedSuccess, true
	case ExecutionContextDrift, ExecutionRedirectedFeed:
		return VerifContextDrift, true
	case ExecutionTargetNotReached:
		return VerifTargetNotReached, true
	case ExecutionRateLimited:
		return VerifRateLimited, true
	case ExecutionBlocked:
		return VerifBlocked, true
	case ExecutionCaptcha:
		return VerifCaptcha, true
	case ExecutionShadowRejected:
		return VerifShadowRejected, true
	case ExecutionComposerFailed,
		ExecutionVerificationTimeout,
		ExecutionRetryExhausted,
		ExecutionHardFail,
		ExecutionSoftFail:
		return VerifExecutionFailed, true
	default:
		return VerifExecutionFailed, false
	}
}

// IsVerifiedSuccess is the predicate that gates engagement_events
// emission (PR-2). Single source of truth — DO NOT inline this check
// elsewhere, always call through this function so future taxonomy
// changes don't require hunting callsites.
func IsVerifiedSuccess(state ExecutionState, outcome VerificationOutcome) bool {
	return state == ExecFinished && outcome == VerifVerifiedSuccess
}

// IsTerminal reports whether the execution_state is in a terminal
// position (finished or expired). Used by claim / reset paths to
// decide whether a row is still actionable.
func IsTerminal(state ExecutionState) bool {
	return state == ExecFinished || state == ExecExpired
}

// LegacyStatusFor and ExecutionStateFromLegacyStatus were removed in
// PR-2 (V2 staged refactor 2026-05-20). The DB column `status` and
// the Go type OutboundStatus are gone; readers and writers operate on
// ExecutionState + VerificationOutcome directly. Backfill from the
// historical `status` column happens in schema v7's data migration.
