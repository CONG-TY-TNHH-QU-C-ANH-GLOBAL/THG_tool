# Fresh-Lead Discovery — Operator Experience

Domain: **facebook-sales-intelligence**. Layer: **experience** for the
`fresh-lead-discovery` experience. Extracted from the PR-M0/PR-C0.5 specs
(now [multi-group-fresh-lead-crawl/technical.md](../../features/multi-group-fresh-lead-crawl/technical.md)
and [account-safety/technical.md](../../features/account-safety/technical.md)).
Status: **draft — docs only.**

## Operator flow

1. The operator defines a **campaign**: which groups, the freshness window
   (default 24h), which of their accounts may serve it, and how often sources
   become due.
2. The campaign compiles into a queue of per-group runs, admitted one at a time
   per account through the safety machinery. The operator watches run progress
   through the existing crawl-progress telemetry.
3. Fresh posts become leads; everything else is excluded with a visible, typed
   reason.

## Visible states and transparency

- **"Why did 40 posts produce 3 leads?"** Counters for every exclusion reason
  (`stale_post`, `timestamp_unparsed`, `timestamp_ambiguous`,
  `timestamp_invalid`, `duplicate_lead` — defined in
  [technical.md §3](../../features/multi-group-fresh-lead-crawl/technical.md))
  ride the crawl-progress telemetry, so a low lead count is explained, never
  mysterious. Lower lead volume per crawl is the designed outcome, not a bug.
- **Run history is append-only**: every run ends with a typed
  `exit_reason_code` the operator can see — the operator sees the reason, never
  a silent stall. Stopping at the temporal frontier ("feed exhausted of fresh
  posts") is reported as a successful early exit.
- **Per-account safety states are surfaced with operator messages** (queued,
  running, cooling down, checkpoint/login walls, risk blocks) — the message
  catalog lives with the state machine in
  [account-safety/technical.md §3](../../features/account-safety/technical.md).

## Errors, recovery, and human-required behavior

- **Checkpoint / login walls stop the run safely** and move the account to
  `checkpoint_required` / `login_required` → `human_required`. The system never
  solves, bypasses, or rotates to another account to route around a wall; the
  source waits for the same account to recover.
- **`human_required` clears only through the operator path** (resolve via the
  checkpoint alert + VNC deep-link, verifier-confirmed). No timer or scheduler
  auto-clears it, even if that stalls a campaign.
- **A dead account never blocks the campaign**: other pool accounts keep
  draining the queue; the affected source waits visibly.

## Connector upgrade wall

When a fresh-lead campaign requires a connector version that can prove post
freshness and no eligible connector supports it, the run is held in
**`waiting_for_connector_upgrade`** — a first-class, operator-visible run
status. It is never silently dispatched to produce zero leads (which would be
indistinguishable from "no fresh posts exist"). Legacy (non-campaign) crawls on
the same connector keep working unchanged.

## Supporting technical features

- [multi-group-fresh-lead-crawl](../../features/multi-group-fresh-lead-crawl/technical.md)
- [account-safety](../../features/account-safety/technical.md)
- Platform dependency: browser-connector
  (`specs/domains/platform-foundation/features/browser-connector/runbooks/connector-production-workflow.md`,
  `specs/domains/platform-foundation/features/browser-connector/technical.md`).
