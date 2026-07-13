package crawlrun

import "time"

// Fence identifies one append-only run attempt. Every fenced mutation
// (heartbeat, recover) matches org_id + run_id + attempt + status='running', so
// a stale worker's write hits zero rows instead of touching a newer attempt.
type Fence struct {
	OrgID   int64
	RunID   int64
	Attempt int
}

func (f Fence) valid() bool {
	return f.OrgID > 0 && f.RunID > 0 && f.Attempt > 0
}

// EnqueueDueRunsInput scopes a due-run enqueue pass to one org at a server-
// authoritative instant.
type EnqueueDueRunsInput struct {
	OrgID int64
	Now   time.Time
}

// EnqueueDueRunsOutcome reports the runs the pass materialized. ReusedRunIDs are
// existing open runs a concurrent enqueue had already created (idempotent
// conflict resolution); CreatedRunIDs are freshly queued rows.
type EnqueueDueRunsOutcome struct {
	CreatedRunIDs []int64
	ReusedRunIDs  []int64
}

// ClaimNextRunInput asks to claim one queued run for a specific eligible
// account. Now is the server clock used to derive the run's fresh cutoff.
type ClaimNextRunInput struct {
	OrgID     int64
	AccountID int64
	Now       time.Time
}

// ClaimedRun is the run transitioned to running by a successful claim.
type ClaimedRun struct {
	RunID         int64
	CampaignID    int64
	SourceID      int64
	AccountID     int64
	Attempt       int
	FreshCutoffAt time.Time
}

// HeartbeatOutcome is the typed result of a fenced heartbeat. Every non-match —
// wrong org, wrong run, wrong attempt, or a non-running/terminal status — folds
// into HeartbeatStaleRejected, so the store never lets a caller distinguish (and
// thereby probe) another tenant's runs or a newer attempt.
type HeartbeatOutcome string

const (
	HeartbeatUpdated       HeartbeatOutcome = "updated"
	HeartbeatStaleRejected HeartbeatOutcome = "stale_rejected"
)

// dispatchFailedReason is the default exit_reason_code stamped on a run whose
// command dispatch failed after claim.
const dispatchFailedReason = "dispatch_failed"

// RecoverResult is the typed outcome of a dispatch-failure recovery attempt.
type RecoverResult string

const (
	RecoverRecovered        RecoverResult = "recovered"
	RecoverAlreadyRecovered RecoverResult = "already_recovered"
	RecoverStaleAttempt     RecoverResult = "stale_attempt"
	RecoverParentNotRunning RecoverResult = "parent_not_running"
)

// RecoverDispatchFailureInput fences the recovery to one claimed attempt.
// ReasonCode defaults to dispatchFailedReason when empty.
type RecoverDispatchFailureInput struct {
	Fence             Fence
	ExpectedAccountID int64
	Now               time.Time
	ReasonCode        string
}

// RecoverDispatchFailureOutcome carries the result and, when a retry was
// created or reused, its run id.
type RecoverDispatchFailureOutcome struct {
	Result     RecoverResult
	RetryRunID int64
}
