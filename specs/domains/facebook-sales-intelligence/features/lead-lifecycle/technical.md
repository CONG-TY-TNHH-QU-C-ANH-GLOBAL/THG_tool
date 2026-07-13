# Lead Lifecycle + Work Queue + Auto Archive

**Track:** Comment Intelligence / Sales Copilot (lead read-model + planner input).
**Status:** PR-1..PR-5 implemented (work_queue.go, lead_archive.go, auto_archive_scheduler.go, server/leads/lifecycle.go, LifecycleTabs.tsx, copilot_wording.go all shipped); evidence-retention compaction remains config-staged, not executing.
**Design-doc-first** per V2 mandate (`feedback_v2_tenant_isolation_mandates`).

## Problem

The crawler produces a high volume of leads. If the dashboard shows every raw/old
lead, three things go wrong:

1. **UX** — the operator drowns in stale rows and can't see what to act on now.
2. **Planner correctness** — "comment 5 leads" can pick cold, long-dead leads.
3. **DB growth** — raw crawl payload + evidence accumulate without bound.

We need an explicit lead **lifecycle**, a dashboard-oriented **work queue** the
planner selects from, and a safe **auto-archive + retention** policy. No hard
delete of business truth.

## Core architectural decision: projection vs stored state

This system's truth model is already settled by the locks:

- `feedback_verified_state_centric` — the engagement ledger (`action_ledger`) is
  business truth; downstream **never** reads `outbound_messages.status`.
- `feedback_deterministic_boundaries` — branch on **explicit fields**, no proxy state.
- `feedback_append_only_correction_events` — the ledger is immutable.

Therefore the lifecycle fields split into two kinds:

| Field | Kind | Source |
|---|---|---|
| `freshness_state` | **projected** | pure function of the signals below |
| `next_action`, `next_action_at` | **projected** | pure function |
| `last_engaged_at` | **projected** | verified `action_ledger` touch (existing engagement projection) |
| `last_customer_reply_at` | **projected** | `conversation_threads.last_inbound_at` |
| `last_crawled_at` | **projected** | `leads.created_at` (ingest time) |
| `last_seen_at` | **projected** | `max(last_crawled_at, last_engaged_at, last_customer_reply_at)` |
| `archived_at` | **stored** | explicit archive decision (operator or job) |
| `archive_reason` | **stored** | typed reason code for the archive decision |

**Rationale.** Freshness/next-action are derived truth — storing them would
duplicate the ledger and drift (the exact mistake `verified_state_centric`
forbids). Archiving is *not* derivable: it is a new, reversible decision a human
or the maintenance job makes. So it — and only it — is persisted, on the
canonical `leads` table (the ingest pipeline mirrors `task_leads → leads`, and
the dashboard reads `leads`; see `internal/leadingest/ingest.go`).

This mirrors the two existing projections — `LeadEngagementState` (badge) and
`LeadCoverageState` — both pure functions over the ledger. `LeadLifecycleState`
is a **third orthogonal projection** on the work-management axis, not a
replacement for the coordination badge (`project_service_state_machine`: two
state machines is an established pattern here).

## Lifecycle state machine (`freshness_state`)

Pure function `models.DeriveLeadLifecycle(inputs, policy, now)`. Order matters;
first match wins. All inputs are explicit timestamps — no proxy checks.

```
archived        archived_at set                                   (stored, wins first)
active          customer replied (reply after our last touch)     → needs our response
active          untouched AND age < stale_after_days              → fresh, eligible to act
waiting_reply   we touched, no reply, within followup window       → give them time
followup_due    we touched, no reply, past followup window,        → re-engage now
                  but last activity < stale_after_days
stale           no meaningful activity ≥ stale_after_days          → cold, archive candidate
```

`last_activity = max(last_crawled_at, last_engaged_at, last_customer_reply_at)`.

### `next_action` / `next_action_at` (drives the work-queue ordering in PR-2)

| freshness_state | next_action | next_action_at |
|---|---|---|
| active (untouched) | `comment` | now |
| active (replied) | `reply` | now (customer waiting) |
| waiting_reply | `wait` | last_engaged_at + followup window |
| followup_due | `followup` | now |
| stale | `archive` | now |
| archived | `none` | zero |

## Config defaults (`LeadLifecyclePolicy`)

| Knob | Default | Env |
|---|---|---|
| `stale_after_days` | 14 | `LEAD_STALE_AFTER_DAYS` |
| `archive_after_days` | 30 | `LEAD_ARCHIVE_AFTER_DAYS` |
| `evidence_retention_days` | 14 | `LEAD_EVIDENCE_RETENTION_DAYS` |
| `raw_crawl_retention_days` | 90 | `LEAD_RAW_CRAWL_RETENTION_DAYS` |
| `followup_window` | 24h | reuses `models.DefaultFollowupWindow` |

## Archive reason codes (typed, stable strings)

```
cold_no_activity        cold / no activity after stale window
coverage_full_no_reply  max coverage reached and no reply after archive window
invalid_target_url      target URL invalid / unresolvable
manual_not_relevant     operator marked not relevant
thread_inactive         conversation thread inactive too long
```

## PR plan

- **PR-1 (done)** — Backend lifecycle model. Migration adds `archived_at` +
  `archive_reason` to `leads`. Pure `models.DeriveLeadLifecycle` + `LeadLifecyclePolicy`.
  Store: `GetLeadLifecycle`, `ArchiveLead`, `UnarchiveLead`. `GetLeadsFiltered`
  excludes archived → default list + planner both stop selecting archived. No hard
  delete. Engagement ledger untouched, so dedup/coverage history keeps archived leads.
- **PR-2** — Work Queue projection. A read model returning only `active` /
  `followup_due` by default (excl. archived/stale unless requested), ordered by
  score → freshness → `next_action_at`, carrying coverage state + actor-specific
  touch status. Planner reads from the work queue, not the raw lead list.
- **PR-3 (done)** — Auto-archive job. `models.EvaluateArchive` (pure) + per-org
  `leads.ArchiveSweep` + `runAutoArchiveScheduler` (periodic, mirrors the crawl
  scheduler) wired in `cmd/scraper`. Archives cold, coverage-full-no-reply,
  invalid-target, and replied-then-inactive leads with typed reasons. Config
  defaults above are loaded from env. Append-only ledger preserved (archiving only
  flips `archived_at`).
  - **Deferred (config-staged, not yet executing):** evidence/raw-crawl
    retention *compaction* (the `purged` sub-state — deleting
    `execution_attempts.evidence_json` blobs and raw `task_leads` payload past
    `evidence_retention_days` / `raw_crawl_retention_days`). The knobs exist and
    flow into the policy, but the destructive deletion is intentionally NOT
    implemented yet: it needs its own design pass to guarantee it never touches the
    append-only ledger or the lead skeleton used for dedup/coverage history. Archiving
    (reversible, non-destructive) ships now; compaction (irreversible) waits.
- **PR-4** — Frontend. Lifecycle tabs (Cần xử lý / Chờ phản hồi / Đến hạn
  follow-up / Đã lưu trữ) + freshness/coverage/reply/archived filters. Default
  view hides archived + stale.
- **PR-5** — Copilot wording. "comment N leads" selects from active eligible
  leads and reports scanned/queued/skipped by lifecycle reason; when none
  eligible, suggests crawl-more / enable follow-up / view archived.

## Closeout hardening (post PR-1..PR-5)

- **Auto-archive runtime safety.** Wired at server startup
  (`cmd/scraper/main.go` → `runAutoArchiveScheduler(ctx, db, cfg)`). Defaults:
  `stale_after_days=14`, `archive_after_days=30`, sweep cadence `360 min`. The sweep
  is tenant-scoped (`ArchiveSweep` requires `org_id>0`; the scheduler iterates
  `OrgIDsWithActiveLeads`). Each org sweep emits a typed metric on the event taxonomy:
  `events.LeadArchiveSweep` with `org_id`, `scanned_count`, `archived_count`,
  `archive_reason_counts`, `duration_ms`. No hard delete (flips `archived_at` only).
- **Reversible archive.** `leads.UnarchiveLead` (sets `archived_at=NULL`,
  `archive_reason=''`) + `POST /api/leads/:id/unarchive` + frontend
  `useArchivedLeads().restore`. Archive/unarchive are not separately ledger-audited
  (the leads table is the mutable registry, not the append-only ledger); the sweep
  metric above is the archive audit trail. Manual archive: `POST /api/leads/:id/archive`.
- **Explicit archived override.** Default planner + work queue EXCLUDE archived. A
  deliberate request opts in via `WorkQueueOptions.IncludeArchived` (surfaces archived
  leads with `freshness_state=archived` + reason). The PLANNER path
  (`WorkQueueLeads`) never sets it — re-engaging an archived lead requires an explicit
  unarchive first. The copilot's no-eligible message already names the archived count
  and points to the archived view ("xem mục đã lưu trữ").
- **`commented` flag fixed (was a dead subquery).** `GetLeadsFiltered` previously
  read `outbound_messages.status='sent'` — a column the taxonomy split stopped
  writing, so the flag was always false. It now reads verified `action_ledger`
  (`outcome='succeeded'`) truth. Guarded by a no-drift test.
- **Frontend decomposition.** LeadsView (god view) is not to grow further; split plan
  in `specs/domains/platform-foundation/features/workspace-ui/implementation/leadsview-decomposition.md` (refactor-only). Deferred lifecycle UI (filter
  chips, restore button) ships after the split.

## Invariants preserved

- Tenant isolation: every new query filters `org_id = ?`.
- Append-only ledger: archiving writes only the `leads` table (mutable registry),
  never `action_ledger` / `execution_attempts`.
- No hard delete in PR-1..PR-2; PR-3 compaction removes only raw crawl/evidence
  blobs, never the ledger or the lead skeleton used for dedup/coverage history.
- File size: every new file ≤ 200 lines; god files (`models.go`, `leads.go`,
  `lead_engagement.go`, `config.go`) get minimal additive edits only.
