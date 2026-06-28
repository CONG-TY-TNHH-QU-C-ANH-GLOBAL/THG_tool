---
id: ARCHCM-R2
status: BLOCKED
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: "audit/archcm-r2-crawl-runtime-semantics"
pr_url: ""
boundary_target: blocked-decision
audit_status: COMPLETE
---

# ARCHCM-R2 — AUDIT: crawl runtime / dispatch semantics

## Goal (audit-only)
Document the crawl_runtime.go fallback chain (open crawl → account resolve →
connector dispatch → jobStore fallback): resumability, race conditions on
account-offline mid-submit, RBAC of pickReadyFacebookAccountIDForCrawl. Gates ARCHCM4.

## Component / domain
crawler/jobhandler runtime + connector dispatch. RED.

## Dependencies
Blocks ARCHCM4.

---

# AUDIT RESULT (2026-06-28 — semantics documented; the 3 questions answered)

## 0. The dispatch chain (as built)
`submitOpenCrawl` (crawl_runtime.go:19): `resolveCrawlAccountID` → build `jobs.Task`
(deterministic `TaskID`, `RetryPolicy{MaxAttempts:3, BackoffMs:1000}`) →
`rememberRecurringCrawlIntents` (if `interval_minutes>0`) → `submitConnectorCrawl`;
if it routes, return; else `jobStore.Submit` (server-side fallback).

`submitConnectorCrawl` (:217) dispatch ladder — FIRST match wins, `routed=true`
short-circuits:
1. `task==nil || OrgID<=0 || AccountID<=0` → **not routed** → server job.
2. Fresh (`≤5min`) FB-logged-in connector **screenshot** for the account →
   `enqueueConnectorCrawlCommand(screen.AgentID)` → routed.
3. `pickOnlineConnectorForCrawl` (shared `connectors.PickReadyConnector` eligibility)
   → enqueue(agentID) → routed; else capture the typed `connectorReason`.
4. CDP **AppStore session** (`CDPPort>0`, status idle/ready/active) → **not routed**
   → server job.
5. else → routed with an operator error ("extension not online", + precise reason).

`enqueueConnectorCrawlCommand` (:311): `CreateTask`+`StartTask` → build envelope
(refuse if no concrete source URL — prevents newsfeed fallback) → `CreateConnectorCommand`
(durable row the extension polls). Envelope/command error → `FailTask`.

Recurring spine (crawl_scheduler.go): `runCrawlIntentScheduler` ticks (default 1m) →
`scheduleDueCrawlIntents` → `ClaimDueIntents(now, 10)` (claim-based, `status='active'`)
→ deterministic `recurringCrawlTaskID = autocrawl-{intentID}-{unix/bucketSeconds}` →
`submitOpenCrawl` → `MarkIntentRunResult`.

## Q1 — Resumability of the fallback chain
- **Recurring path: resumable (claim-based).** `ClaimDueIntents` re-selects active+due
  intents every tick; a transient `submitErr` is recorded via `MarkIntentRunResult`
  but the intent **stays active** → retried next due tick. The time-bucketed
  `recurringCrawlTaskID` is an idempotency key: re-firing within the same interval
  bucket yields the SAME task id. A **permanent** misconfig (`account_id<=0`) is
  terminally `failed` (SetIntentStatus) so it is never re-claimed — no silent
  first-ready fallback (PR-A invariant).
- **Connector path: resumable via the durable command queue.** `CreateConnectorCommand`
  persists a row the extension polls when it next comes online — no re-submit needed
  while it is merely offline. BUT if `submitConnectorCrawl` returns an **error**
  mid-enqueue (e.g. `CreateConnectorCommand` fails), the task is `FailTask`'d and is
  **not auto-retried** for a manual one-shot crawl — the operator re-submits (the
  deterministic open-crawl `TaskID` makes re-dispatch idempotent for the same
  day/sources/account).
- **Server fallback:** `jobStore.Submit` carries `RetryPolicy{3, 1000ms}` → resumable
  via job retry.
- **OPEN QUESTION (out of this file, for connector/jobs owners):** does
  `CreateConnectorCommand` have a TTL / GC? A command for a connector that never
  returns online appears to sit indefinitely (resumable but potentially stale). And is
  `CreateConnectorCommand` idempotent on re-dispatch, or can a manual re-submit create
  a duplicate command row?

## Q2 — Race conditions on account-offline mid-submit
- The "is the connector online" decision is **read at submit time** (screenshot
  freshness `≤5min`, OR `PickReadyConnector` live `Online` flag, OR CDP session status)
  and **acted on** (`CreateConnectorCommand`) with **no lock/CAS** → a TOCTOU window.
- **Widest window = the screenshot route (step 2):** it accepts a screenshot up to
  **5 minutes old**, so a connector offline for ~5 min can still be dispatched to. The
  command then lands on an offline connector and sits unpicked until it returns.
- **No correctness corruption:** the command is a durable row keyed by
  org/account/agent; the race manifests as "queued but not executed" (a liveness /
  latency gap surfaced via operator status), not data loss or misrouting.
- **No intra-submit double-dispatch:** the ladder is sequential and short-circuits on
  the first route, so one submit creates at most one command.
- The CDP-session branch (step 4) returns not-routed → the race is deferred to the
  server job's own readiness handling.

## Q3 — RBAC of pickReadyFacebookAccountIDForCrawl
- **Auto-pick path (`account_id<=0`): owner-filtered.** `resolveCrawlAccountID` calls
  `pickReadyFacebookAccountIDForCrawl`, whose `allow` gate restricts an identified,
  non-privileged sales member to accounts they own (admin/platform + the `userID<=0`
  scheduler stay org-wide). The gate filters connector `AssignedAccountID`,
  `screen.AccountID`, AND the all-accounts loop — so auto-pick CANNOT land a sales
  member on another member's online account (PR-M3 member scope). (Note: this is the
  inline copy of the OWNER role rule; sharing the ARCHCM-R1a
  `callerRestrictedToOwnedAccounts` helper here is the deferred ARCHCM2c/crawl item.)
- **Explicit path (`account_id>0`): NO ownership check (ASYMMETRY).**
  `resolveCrawlAccountID` only owner-filters when `account_id<=0`; an explicit
  `account_id` is used **as-is**. So a crawl with an explicit `account_id` the caller
  does not own proceeds — the PR-M3 member scope that the auto-pick path enforces is
  bypassed on the explicit path.
  - *Rationale today:* crawl is a READ action (no public side-effect); the write-path
    control gate (`canRequesterControlAccount`) deliberately excludes read/crawl/search.
  - *Risk to decide:* a sales member can target ANOTHER member's account for a crawl
    (uses that account's connector/identity for a read). Acceptable for a pure read, or
    should the explicit crawl path also be owner-filtered for consistency? **Founder RBAC
    decision** — it is a behavior question, NOT a refactor.
- **Scheduler path:** recurring intents carry their creator's `account_id`
  (`intent.AccountID`, creation-time). The scheduler runs as system (`userID<=0`, org-wide)
  but pins the account, so a recurring crawl runs on its pinned account even if that
  account is later reassigned (creation-time ownership; minor staleness note).

## Options (what ARCHCM-R2 unlocks for ARCHCM4)
- **Option A (recommended): sign off the semantics; ARCHCM4 is a behavior-preserving
  move guarded by the invariant checklist below — fix nothing here.** The RBAC
  asymmetry (Q3) and the resumability open questions (Q1) are tracked as SEPARATE
  decisions, never bundled into the move.
- **Option B: sign off + commit to closing the Q3 RBAC asymmetry first** (owner-filter
  the explicit `account_id` crawl path) as a small, tested, behavior-CHANGING PR before
  ARCHCM4. Choose this only if the founder decides crawl must be owner-consistent.
- **Option C: defer ARCHCM4** — leave crawl runtime in cmd. The runtime is sensitive
  (connector dispatch + jobs + the 5-min race); if there is no boundary payoff yet,
  deferring is legitimate.

## Recommended default: **Option A**
The dispatch semantics are coherent and the resumability/race behaviors are
intentional (durable command queue + claim-based scheduler + deterministic task ids).
ARCHCM4 may proceed as a **behavior-preserving move** (RED-adjacent) once a human signs
off, preserving the invariants below; the Q3 RBAC asymmetry is a separate founder
decision, not a refactor.

### ARCHCM4 move invariant checklist (must hold byte-for-byte)
1. Dispatch ladder ORDER + first-match short-circuit (steps 1–5).
2. The `≤5min` screenshot freshness window (step 2).
3. not-routed → `jobStore.Submit` server fallback (steps 1 & 4).
4. Deterministic `openCrawlTaskID` / `recurringCrawlTaskID` (idempotency keys).
5. Claim-based scheduler + `account_not_selected` permanent-fail (no first-ready fallback).
6. The auto-pick owner filter (Q3) and the explicit-path pass-through — unchanged.
7. `RetryPolicy{3,1000ms}` on the server task; envelope "no concrete source URL" refusal.
Characterization tests for 1–6 are required before the move (move-only is not a
licence to ship untested runtime logic).

## Validation
N/A (audit — no production code changed).

## Done criteria
Semantics documented + the three audit questions answered (above). ARCHCM4 unblocked
only after human sign-off of Option A (or B). The Q3 RBAC asymmetry is recorded as a
separate founder decision. Stays BLOCKED until sign-off.
