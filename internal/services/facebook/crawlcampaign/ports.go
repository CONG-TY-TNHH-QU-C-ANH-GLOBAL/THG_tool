package crawlcampaign

import (
	"context"
	"time"
)

// This file defines the PR-M4A orchestrator's consumer ports: narrow interfaces,
// each owned by the orchestrator (the consumer) and satisfied by a composition-
// root adapter over the durable store, the Account Safety Coordinator, the
// readiness primitive, and the existing crawl command path. They are kept
// separate on purpose — there is no single FacebookCrawlStore god interface
// (blueprint §13) — so the store never chooses accounts and the orchestrator
// never opens a transaction. The package imports no store/server/cmd package
// (services/facebook reverse-dependency seam); adapters map persistence types to
// these domain DTOs at the composition root.

// PooledClaim is the run one ClaimNextRun transitioned to running, in the
// orchestrator's own vocabulary. Fence is the (org, run, attempt) fencing token
// carried into every dispatch payload (blueprint §9).
type PooledClaim struct {
	Fence         RunFence
	CampaignID    int64
	SourceID      int64
	AccountID     int64
	FreshCutoffAt time.Time
}

// PoolReader lists, per org, the campaign-pool accounts the orchestrator may try
// to claim for. Pool membership only — safety/readiness/budget stay here.
type PoolReader interface {
	ActiveCampaignPools(ctx context.Context) ([]OrgPool, error)
}

// OrgPool is one org and its distinct active-campaign pool accounts.
type OrgPool struct {
	OrgID      int64
	AccountIDs []int64
}

// DueRunEnqueuer materializes queued runs for an org's due sources. Idempotent;
// creates no lease and marks nothing running.
type DueRunEnqueuer interface {
	EnqueueDueRuns(ctx context.Context, orgID int64, now time.Time) error
}

// RunClaimer atomically transitions one eligible queued run to running for a
// specific preselected account. ok is false (nil error) when no run is claimable
// for that account.
type RunClaimer interface {
	ClaimNextRun(ctx context.Context, orgID, accountID int64, now time.Time) (claim PooledClaim, ok bool, err error)
}

// DispatchFailureRecoverer fails a claimed run whose command dispatch failed and
// queues its single retry, in one fenced transaction. Called only after a claim.
type DispatchFailureRecoverer interface {
	RecoverDispatchFailure(ctx context.Context, fence RunFence, accountID int64, now time.Time) error
}

// AccountSafetyGate is the Account Safety Coordinator seen through the
// orchestrator's lens: per-account eligibility (parked/checkpoint/login/risk and
// cooldown fail closed) plus the atomic acquire/release of the one machine crawl
// slot. TryReserve checks the machine budget and marks the account running in a
// single critical section, so a concurrent scheduler can never observe the same
// free slot and double the budget; it returns false when no slot is free. The
// orchestrator reads this gate; it never reimplements safety policy (blueprint
// §7; account-safety technical §Coordinator).
type AccountSafetyGate interface {
	Eligible(accountID int64, now time.Time) bool
	TryReserve(accountID int64, now time.Time) bool
	Release(accountID int64, reason string, now time.Time)
}

// AccountReadinessChecker reports whether an account can dispatch a crawl right
// now (connector online, live identity, supported extension). It gates BEFORE
// the claim so a not-ready account is never claimed-then-recovered every tick —
// that would mint an immediately-claimable retry and become a retry storm.
type AccountReadinessChecker interface {
	Ready(ctx context.Context, orgID, accountID int64) bool
}

// CrawlCommandDispatcher sends one claimed run through the existing Facebook
// crawl command path, carrying the fence in the payload. A non-nil error means
// the command was not accepted; the orchestrator then recovers the run.
type CrawlCommandDispatcher interface {
	Dispatch(ctx context.Context, claim PooledClaim) error
}
