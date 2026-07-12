# Facebook Multi-Group Fresh-Lead Crawl: Code Organization Blueprint

**Target:** `specs/facebook/FACEBOOK_CRAWL_CODE_ORGANIZATION_BLUEPRINT.md`  
**Status:** Companion blueprint for PR-M3 through PR-M5; preflight-gated  
**Track:** Facebook Automation Reliability  
**Authoritative parent:** `specs/facebook/MULTI_GROUP_FRESH_LEAD_CRAWL_SPEC.md`  
**Schema companion:** `specs/facebook/FACEBOOK_CRAWL_CAMPAIGN_POSTGRES_SCHEMA_IMPLEMENTATION.md`

## 1. Purpose

This document defines runtime ownership, dependency direction, atomic ports, transaction boundaries, and rollout seams for later implementation phases.

It does not authorize runtime code in PR-M2B.

The train is intentionally split:

- PR-M2B: PostgreSQL migrations and constraint tests only.
- PR-M3: focused domain types, pure policies, and dormant store operations.
- PR-M4: scheduler/campaign queue wiring behind a default-off feature flag.
- PR-M5: server freshness gate, frontier/result application, lead identity transaction, and campaign progress updates.
- PR-M6: operator UI and controls if still aligned.

## 2. Repository-grounded ownership

Claude must audit current main before creating files. Do not invent textbook directories.

| Concern | Current owner to verify | Ownership rule |
|---|---|---|
| Composition root | `cmd/scraper/main.go` and adjacent runtime files | Wiring only; no business policy |
| Existing scheduler | `cmd/scraper/crawl_scheduler.go` | Thin adapter to campaign orchestration |
| Crawl persistence | `internal/store/crawl/` | PostgreSQL operations and transactions |
| Account safety | `internal/session/accountsafety/` | Eligibility, cooldown, parked/human-required state |
| Session exclusivity | `internal/session/allocator.go` | Account/session lease; no campaign knowledge |
| Result ingest | `internal/server/agent/crawlingest/` | Existing ownership/auth gate; extend, do not duplicate |
| Lead ingest | Verify canonical current owner | Reuse canonical lead creation path |
| Extension timestamp parser | `local-connector-extension/platforms/facebook/crawl_time.js` | DOM parsing and canonical timestamp DTO only |
| Extension orchestration | `local-connector-extension/content/crawl.js` | Thin wiring/orchestration |
| Platform migrations | `internal/store/migrations/platform/` | Durable SaaS schema |

If a path differs on main, record the verified owner and adapt minimally. Do not create `internal/store/postgres/`, `internal/store/ports/`, or `cmd/scraper/wire.go` merely to imitate a generic architecture pattern.

## 3. Dependency direction

Required direction:

```text
transport/composition
        ↓
Facebook campaign orchestration and pure policy
        ↓ narrow consumer-owned ports
store/crawl adapters and existing runtime collaborators
```

Rules:

- `cmd/` and HTTP handlers parse, authorize, route, and wire only.
- Pure policy must not import SQL, HTTP, browser, or DOM packages.
- `internal/store/crawl` must not import the orchestration package.
- Account safety remains owned by `accountsafety`.
- Session lease remains owned by the allocator.
- Campaign account-pool selection must not be added to the allocator.
- DOM timestamp parsing remains in the extension platform module.
- Store code persists already-classified freshness decisions; it does not decide freshness.
- `sql.Tx` must not cross a package boundary.

## 4. Proposed package shape

Use this shape only after confirming that `internal/services/facebook/` is an established service boundary:

```text
internal/services/facebook/crawlcampaign/
    campaign.go
    source.go
    run.go
    statuses.go
    scheduling.go
    freshness.go
    ports.go
    scheduling_test.go
    freshness_test.go

internal/store/crawl/
    campaign_store.go
    run_queue.go
    run_result.go
    lead_identity.go
    *_postgres_test.go

cmd/scraper/
    crawl_scheduler.go
    main.go

internal/server/agent/crawlingest/
    existing result-ingest files
    focused campaign adapter only if the current package needs one
```

If `internal/services/facebook/` is not a current convention, stop and propose the nearest existing service/domain owner instead of creating a parallel architecture.

Do not put domain types and consumer ports in `internal/store/crawl`. The store implements ports; it does not own orchestration policy.

## 5. Strong domain contracts

Use focused, typed contracts rather than weak `string`, `int`, or `[]byte` parameters.

Illustrative types:

```go
type RunStatus string

const (
	RunStatusQueued                     RunStatus = "queued"
	RunStatusWaitingForConnectorUpgrade RunStatus = "waiting_for_connector_upgrade"
	RunStatusRunning                    RunStatus = "running"
	RunStatusSucceeded                  RunStatus = "succeeded"
	RunStatusStoppedSafe                RunStatus = "stopped_safe"
	RunStatusFailed                     RunStatus = "failed"
	RunStatusAbandoned                  RunStatus = "abandoned"
	RunStatusCancelled                  RunStatus = "cancelled"
)

type ExitReason string

type RunFence struct {
	OrgID   int64
	RunID   int64
	Attempt int
}

type CrawlCursor struct {
	LastPostID string
	LastPostAt *time.Time
}

type RunCounters struct {
	PostsSeen       int
	FreshLeadCount  int
	StaleSkipped    int
	DuplicateCount  int
	UnparsedCount   int
}
```

Additional focused types should include:

- `Campaign`;
- `CampaignSource`;
- `CrawlRun`;
- `TimestampParse`;
- `FreshnessDecision`;
- `AccountAssignment`;
- `RunProgress`;
- `RunResult`;
- `ApplyRunResultInput`;
- `ApplyRunResultOutcome`.

Do not create a second canonical `Lead` model. Reuse the existing lead owner.

## 6. Queue creation and atomic claim

Explicitly reject this split:

```go
GetPendingSources(...)
CreateRunAttempt(...)
```

Two schedulers can read the same source before either inserts a run.

Use two idempotent capabilities:

### Enqueue due work

```go
type DueRunEnqueuer interface {
	EnqueueDueRuns(
		ctx context.Context,
		input EnqueueDueRunsInput,
	) (EnqueueDueRunsOutcome, error)
}
```

Contract:

- tenant-scoped;
- determines due sources using persisted campaign/source state;
- inserts queued run rows;
- relies on the one-open-run-per-source index;
- treats unique conflict as already queued, not as an error storm;
- creates no account lease and does not mark a run running.

### Claim an already queued run

```go
type RunClaimer interface {
	ClaimNextRun(
		ctx context.Context,
		input ClaimNextRunInput,
	) (ClaimedRun, bool, error)
}
```

`ClaimNextRunInput` must include:

- `OrgID`;
- selected `AccountID`;
- scheduler/worker identity when the repository uses one;
- current server time;
- required connector capability/version;
- any campaign filter needed by the caller.

Contract:

- uses a PostgreSQL transaction;
- selects a queued/waiting candidate with `FOR UPDATE SKIP LOCKED` or the verified equivalent;
- verifies the account belongs to the campaign pool;
- transitions exactly one run to `running`;
- assigns account and timestamps;
- computes and stores `fresh_cutoff_at` from server time;
- returns no run when none is eligible;
- relies on database active/open constraints as race-condition backstops.

The scheduler must acquire or reserve the existing session/account lease before dispatch. If the database claim fails, release the lease. If command dispatch fails after claim, apply the typed dispatch-failure recovery contract; do not leave a permanently wedged running row.

## 7. Account selection and safety ownership

Campaign orchestration may read allowed account IDs from the campaign pool.

Account assignment policy must then apply:

1. campaign membership;
2. sticky source/account preference;
3. Account Safety eligibility;
4. existing session allocator availability;
5. connector capability/version;
6. machine/org budget;
7. fairness.

The allocator answers lease/availability questions for specific account IDs. It must remain unaware of campaigns and source affinity.

Safety invariants:

- one account has at most one active crawl;
- parked/checkpoint/login/risk accounts are not selected;
- risk stop never triggers automatic account rotation;
- a pinned/sticky source waits for recovery or explicit operator reassignment;
- account-pool membership improves availability and coverage;
- it does not imply same-account concurrency or automatic machine-budget increases.

## 8. Connector capability gate

Strict fresh-lead runs require a connector that supports the canonical timestamp DTO:

- `posted_at`;
- `confidence`;
- `earliest_utc`;
- `latest_utc`.

An unsupported connector must not receive a strict run.

Ownership:

- connector capability reader reports support;
- scheduler transitions or leaves the run as `waiting_for_connector_upgrade`;
- operator telemetry explains the block;
- when a supported connector is available, an idempotent transition returns the run to claimable state;
- no silent dispatch that produces zero leads.

Do not route this with an ad-hoc unvalidated string flag alone. Campaign identity must come from the validated run/task envelope.

## 9. Fenced progress and result writes

Every connector/worker mutation tied to a running attempt must carry:

```go
RunFence{
	OrgID:   ...,
	RunID:   ...,
	Attempt: ...,
}
```

Heartbeat, progress, finish, result, and lead-emission operations must conditionally match:

```text
org_id
run_id
attempt
status = running
```

A stale worker update:

- matches zero rows;
- does not mutate a newer attempt;
- does not create leads;
- produces typed `stale_attempt` or `stale_update_rejected` telemetry.

Do not expose store methods that mutate by `runID` alone.

## 10. Freshness policy ownership

The extension emits the canonical timestamp DTO. The server owns the authoritative freshness decision using server time and the stored `fresh_cutoff_at`.

Pure policy contract:

```go
type FreshnessPolicy interface {
	Evaluate(
		timestamp TimestampParse,
		cutoff time.Time,
		now time.Time,
	) FreshnessDecision
}
```

Rules:

- exact timestamps compare `posted_at`;
- derived-relative timestamps compare the conservative `earliest_utc`;
- ambiguous, unknown, or invalid/future timestamps are not eligible;
- policy returns typed exclusion reasons;
- the store receives an already-classified decision.

Reject store methods named like `AdmitLeadIfFresh` when they combine policy and persistence ambiguously.

## 11. Atomic result application

Do not split one business invariant across unrelated calls such as:

```go
CompleteRun(...)
AdmitLead(...)
AdvanceCursor(...)
```

Define one atomic persistence capability:

```go
type RunResultStore interface {
	ApplyRunResult(
		ctx context.Context,
		input ApplyRunResultInput,
	) (ApplyRunResultOutcome, error)
}
```

Inside one PostgreSQL transaction, the implementation must:

1. verify the `RunFence` and running status;
2. reject stale attempts before any lead mutation;
3. reserve each eligible `(org_id, platform, post_dedup_hash)`;
4. create canonical leads only for successful reservations;
5. attach lead IDs to identity reservations;
6. update typed run counters;
7. finish the run with a typed terminal status and exit reason;
8. update source cursor/last-run state;
9. update campaign aggregates only when those aggregates are part of the approved runtime model;
10. commit.

On error, roll back all database mutations.

After commit, orchestration may:

- release the session/account lease;
- feed the terminal result into Account Safety;
- nudge the scheduler for fast handoff;
- emit operator notifications.

Cross-package side effects must be idempotent or derived from the committed run result. Do not hold a database transaction open while calling browser, network, Telegram, or other external services.

## 12. Result-ingest integration

Extend the existing `internal/server/agent/crawlingest` ownership and authorization gates.

Do not create a parallel endpoint that duplicates:

- connector authentication;
- org/account ownership checks;
- task ownership;
- payload validation;
- result idempotency.

The existing path should detect a validated campaign-run envelope containing `run_id`, `attempt`, and campaign metadata, then delegate to the atomic `RunResultStore`.

Legacy results without a campaign-run envelope continue through the legacy path.

## 13. Narrow ports

Create interfaces only where they protect a real boundary or test seam. Candidate capabilities:

- `CampaignReader`;
- `DueRunEnqueuer`;
- `RunClaimer`;
- `RunProgressStore`;
- `RunResultStore`;
- `ConnectorCapabilityReader`;
- `CrawlCommandDispatcher`;
- `AccountEligibilityReader`;
- `AccountLease`;
- optional scheduler nudge only when justified.

For each interface, document:

- consumer/owner;
- existing implementation to adapt;
- transaction expectation;
- org/fence requirements;
- whether it exists only for testing or is a real volatile boundary.

Do not create one `FacebookCrawlStore` god interface. Do not create an interface solely because dependency injection is fashionable.

## 14. Runtime PR sequence

### PR-M3 — dormant domain/store foundation

- typed statuses, reasons, fences, inputs, and outcomes;
- pure freshness policy;
- due-run enqueue and atomic claim store operations;
- fenced progress/result transaction;
- real PostgreSQL store tests;
- no production scheduler or ingest wiring.

### PR-M4 — scheduler and account-pool wiring

- default-off feature flag;
- campaign due-run enqueue;
- account-pool selection;
- Account Safety eligibility;
- session allocator lease;
- connector capability gate;
- claim and command dispatch;
- legacy scheduler retained.

### PR-M5 — fresh-lead/result behavior

- server-defined `now_utc` and `fresh_cutoff_at`;
- frontier behavior in the approved extension/runtime layer;
- canonical DTO ingest;
- atomic lead identity claim;
- run/source/campaign updates;
- fast handoff after terminal result;
- operator telemetry.

### PR-M6 — operator UX

- campaign controls;
- queue/run/source states;
- connector-upgrade and account-risk visibility;
- aggregate fresh/stale/duplicate counters.

Do not collapse these phases into a big-bang PR.

## 15. Production cutover

Feature flag must be default off and preferably org-scoped, with optional campaign-level enablement.

Before canary:

- migrations applied and verified;
- PR-M3 store tests green;
- legacy path regression tests green;
- metrics and operator visibility available;
- internal test org selected;
- connector capability confirmed.

Prevent duplicate scheduling by giving one source exactly one active owner:

- a legacy `org_crawl_intent` source remains legacy-owned; or
- it is explicitly migrated/disabled before campaign ownership begins.

Do not run legacy and campaign schedulers against the same normalized source concurrently.

Recommended rollout:

1. internal org, one account, a few groups;
2. verify sequential handoff and no duplicate leads;
3. test checkpoint/login park behavior;
4. enable a small account pool with machine budget still one;
5. observe queue lag, time between targets, stale skips, and account safety;
6. expand by org.

Rollback:

- disable the feature flag;
- stop new claims;
- allow or safely terminate already-running attempts according to account-safety policy;
- preserve database history;
- leave legacy path available;
- do not drop schema as an application rollback.

When PostgreSQL is unavailable, campaign scheduling fails closed. Do not silently fall back to a local SQLite queue or legacy source for the same campaign.

## 16. Testing strategy

### Pure policy

- timestamp confidence and cutoff boundaries;
- account-assignment ordering;
- connector capability transitions;
- status/exit-reason mappings.

### PostgreSQL store

- due-run enqueue idempotency;
- `FOR UPDATE SKIP LOCKED` claim races;
- one active account;
- one open source;
- retry lineage;
- task idempotency;
- fenced progress/result writes;
- stale-attempt rejection;
- atomic rollback;
- lead identity concurrency;
- tenant isolation.

### Scheduler integration

- one account plus five groups runs sequentially;
- next target starts after terminal feedback;
- multiple eligible accounts distribute only when explicit budgets permit;
- parked account skipped;
- sticky source waits after risk;
- dispatch failure releases or recovers lease/run safely;
- unsupported connector produces `waiting_for_connector_upgrade`.

### Ingest

- legacy payload remains unchanged;
- campaign envelope uses fenced path;
- duplicate result is idempotent;
- stale attempt creates no leads;
- auth and ownership gates run before mutation.

### Cutover

- default-off behavior;
- org-scoped canary;
- same source cannot be owned by legacy and campaign schedulers;
- disabling flag stops new claims without deleting history.

## 17. Rejected designs

- `GetPendingSources` followed by `CreateRunAttempt`.
- Mutation by `run_id` without org and attempt fence.
- Generic `reason string`, `items int`, or opaque cursor bytes across core boundaries.
- Freshness decisions in the SQL/store adapter.
- `sql.Tx` leaked to service or transport.
- One god `FacebookCrawlStore`.
- Generic scheduler framework not required by this feature.
- Domain types owned by the store package.
- Campaign logic added to the session allocator.
- Parallel result endpoint duplicating existing auth/ownership gates.
- Big-bang replacement of the legacy scheduler.
- Same-account concurrent crawling.
- Risk-triggered account rotation.
- SQLite fallback for durable campaign state.

## 18. Acceptance criteria

This blueprint is ready to guide PR-M3 through PR-M5 only when:

- actual repository owner paths are verified before file creation;
- domain types and ports are not placed in the store package;
- due-run creation and run claim are atomic/idempotent;
- all important mutations are tenant-scoped and fenced;
- result application has one explicit transaction boundary;
- freshness policy is separate from persistence;
- Account Safety, allocator, connector capability, and campaign ownership are distinct;
- legacy/campaign dual ownership is prevented;
- feature flag, canary, fail-closed, and rollback behavior are actionable;
- tests map to race, tenant, fencing, idempotency, and cutover risks.
