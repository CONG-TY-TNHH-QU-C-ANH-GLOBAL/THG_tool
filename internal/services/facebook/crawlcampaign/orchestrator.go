package crawlcampaign

import (
	"context"
	"time"
)

// dispatchFailedReason is the machine-slot release reason after a dispatch
// failure; it mirrors the store's terminal exit_reason_code for the same event.
const dispatchFailedReason = "dispatch_failed"

// Deps are the PR-M4A orchestrator's collaborators, all consumer ports. Logf is
// optional (nil = silent); every other port is required and supplied by the
// composition root.
type Deps struct {
	Pools      PoolReader
	Enqueuer   DueRunEnqueuer
	Claimer    RunClaimer
	Recoverer  DispatchFailureRecoverer
	Safety     AccountSafetyGate
	Readiness  ReadinessGate
	Dispatcher CrawlCommandDispatcher
	Logf       func(format string, args ...any)
}

// Orchestrator turns due campaign/source work into at most one safely fenced
// Facebook crawl dispatch per machine slot, per tick. It owns account-selection
// policy and ordering; the durable mechanics (enqueue/claim/recover) and the
// Account Safety decision live behind its ports. It never opens a transaction,
// never chooses freshness, and never applies a run result (that is the M3C
// ApplyRunResult boundary, consumed by PR-M5 ingest — not here).
type Orchestrator struct {
	d Deps
}

// New constructs the orchestrator from its ports.
func New(d Deps) *Orchestrator { return &Orchestrator{d: d} }

// RunOnce executes one orchestration pass at the server-authoritative instant
// now: enqueue due work per org, then for each pool account in eligibility order
// select → claim → dispatch → (on failure) recover, bounded by the machine crawl
// budget. A per-org failure is logged and skipped so one org never stalls the
// tick; only the top-level pool read propagates.
func (o *Orchestrator) RunOnce(ctx context.Context, now time.Time) error {
	pools, err := o.d.Pools.ActiveCampaignPools(ctx)
	if err != nil {
		return err
	}
	for _, pool := range pools {
		o.runOrg(ctx, pool, now)
	}
	return nil
}

// runOrg materializes the org's due runs, then walks its pool accounts applying
// the blueprint §7 selection order: campaign membership (the pool) → sticky
// preference and pool membership (enforced durably inside the claim) → Account
// Safety eligibility → connector/account readiness → machine budget. It stops as
// soon as the machine budget is spent.
func (o *Orchestrator) runOrg(ctx context.Context, pool OrgPool, now time.Time) {
	if err := o.d.Enqueuer.EnqueueDueRuns(ctx, pool.OrgID, now); err != nil {
		o.logf("crawlcampaign: enqueue org=%d: %v", pool.OrgID, err)
		return
	}
	for _, accountID := range pool.AccountIDs {
		if o.d.Safety.FreeSlots(now) <= 0 {
			return // machine crawl budget spent — remaining work waits for a later tick
		}
		if !o.d.Safety.Eligible(accountID, now) {
			continue // parked/checkpoint/login/risk/cooldown fails closed — never auto-rotate
		}
		if !o.d.Readiness.Ready(ctx, pool.OrgID, accountID) {
			continue // no connector/identity/supported extension — do not claim, avoids retry storm
		}
		o.claimAndDispatch(ctx, pool.OrgID, accountID, now)
	}
}

// claimAndDispatch claims one run for the preselected account and dispatches it.
// The machine slot is reserved only after a successful claim (we hold a running
// run), and released after a dispatch failure only once the durable recovery has
// committed — the lease is never dropped before the DB reflects the failure.
func (o *Orchestrator) claimAndDispatch(ctx context.Context, orgID, accountID int64, now time.Time) {
	claim, ok, err := o.d.Claimer.ClaimNextRun(ctx, orgID, accountID, now)
	if err != nil {
		o.logf("crawlcampaign: claim org=%d account=%d: %v", orgID, accountID, err)
		return
	}
	if !ok {
		return // no queued run this account may serve
	}

	o.d.Safety.Reserve(accountID, now)
	if err := o.d.Dispatcher.Dispatch(ctx, claim); err != nil {
		o.logf("crawlcampaign: dispatch org=%d run=%d account=%d: %v", orgID, claim.Fence.RunID, accountID, err)
		if rerr := o.d.Recoverer.RecoverDispatchFailure(ctx, claim.Fence, accountID, now); rerr != nil {
			// Recovery failed: leave the run 'running' and keep the slot held. The
			// Coordinator's stale-run timeout returns the slot; a later tick / the
			// reaper reconciles the run. Never release before the DB reflects it.
			o.logf("crawlcampaign: recover org=%d run=%d account=%d: %v", orgID, claim.Fence.RunID, accountID, rerr)
			return
		}
		o.d.Safety.Release(accountID, dispatchFailedReason, now)
		return
	}
	// Dispatched. The run stays 'running' under its fence; freeing the slot on the
	// reported result is PR-M5 result ingest. Until then the Coordinator's stale
	// timeout is the conservative slot return.
	// ponytail: precise result-feedback → M5; stale-timeout ceiling holds meanwhile.
}

func (o *Orchestrator) logf(format string, args ...any) {
	if o.d.Logf != nil {
		o.d.Logf(format, args...)
	}
}
