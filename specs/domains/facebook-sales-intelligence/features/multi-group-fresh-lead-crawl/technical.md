# Multi-Group Fresh-Lead Crawl — Technical Contract (PR-M0)

Track: **Facebook Automation Reliability**. Type: **architecture / code-spec baseline.**
Status: **draft — docs only, no runtime change.** Layer: **technical contract** for the
`multi-group-fresh-lead-crawl` feature (domain: facebook-sales-intelligence).
Business/experience layers: [business.md](../../experiences/fresh-lead-discovery/business.md),
[experience.md](../../experiences/fresh-lead-discovery/experience.md).
Companion to [Account Safety](../account-safety/technical.md) (PR-C0.5) and the
[Crawl Speed & Checkpoint Audit](./evidence/crawl-speed-checkpoint-audit.md) (PR-C0).

This spec defines how one org crawls **many Facebook groups** with a **pool of
accounts** to harvest **fresh leads only** (posts younger than a server-defined
cutoff, default 24h). It layers a campaign/queue orchestration model on top of the
Account Safety Coordinator from PR-C0.5 — it does **not** replace or weaken any
safety rule there. Explicitly out of scope, as in every crawl spec: fingerprint
spoofing, stealth/evasion, proxy/account rotation to dodge checkpoints,
CAPTCHA/checkpoint solving, and speed increases.

Grounding (already in the codebase — reuse, do not reinvent):
- Extension crawl loop: `local-connector-extension/content/crawl.js`
  (`crawlVisibleFacebookPosts`) — has in-run dedup (`dedupKey` + `seen` Set) but
  emits `posted_at: ''` for every item: **post timestamps are not extracted today**,
  so no freshness rule can be enforced yet.
- Server dedup: `posts.dedup_hash UNIQUE` + `INSERT OR IGNORE`
  (`internal/store/crawl/posts.go`), mirrored in platform PostgreSQL
  (`internal/store/migrations/platform/0108_platform_crawl__postgres.up.sql`).
- Recurring crawl cursor: `org_crawl_intents` (`internal/store/crawl/intents.go`)
  with `cursor_last_post_id`, `cursor_last_post_at`, `next_run_at` — single-source
  intents; there is no multi-group campaign grouping today.
- Per-account lease: `session.Allocator.Acquire(accountID, PolicySticky, workerID)`
  (`internal/session/allocator.go`).
- Account-runtime state machine, risk budgets, cooldown, machine concurrency budget:
  PR-C0.5 §3–§5. This spec consumes those as given.

---

## 1. Problem statement

Moved to the experience layer:
[fresh-lead-discovery/business.md](../../experiences/fresh-lead-discovery/business.md)
owns the business problem, intended users, value, and lead-volume goals. In one
line: an org wants "leads from the last 24 hours across my 20 groups, every few
hours", and today's independent `org_crawl_intents` cannot express that. The
answer is a **campaign**: a durable, org-scoped plan (groups + freshness window
+ account pool) compiled into a FIFO queue of per-group **runs**, admitted one at a
time per account through the existing safety machinery, each run stopping early at
the **temporal frontier** (feed exhausted of fresh posts) instead of grinding to
`max_items`.

---

## 2. Orchestration model

```text
facebook_crawl_campaign (org-scoped plan: sources, freshness window, account pool)
   │ compiles into
   ▼
facebook_crawl_runs queue (one queued run per due source, FIFO by priority)
   │ admitted by scheduler: free account from pool + Coordinator budget + lease
   ▼
one run = one account × one group visit, bounded by fresh_cutoff_at + max_items
   │ posts stream through existing crawl-progress path
   ▼
fresh-lead gate at ingest (server): eligible under the §4 confidence gate
```

Roles:
- **Campaign** — the durable plan. Which groups, what freshness window (default
  24h), which accounts may serve it, how often sources become due.
- **Run** — one bounded visit of one source by one account. The unit of queueing,
  leasing, telemetry, and failure handling. Runs are append-only history.
- **Scheduler** — a pure decision function (extends PR-C0.5 `nextAccountToRun`):
  given queue + account states + budgets, pick `(run, account)` or nothing. All
  admission rules from PR-C0.5 §4 apply unchanged.
- **Fresh-lead gate** — server-side ingest policy: only posts proven fresh become
  leads. The extension merely *stops early* on staleness; the server is the
  authority on what becomes a lead.

### Account pool scheduling — safety, not evasion

- **1 account = max 1 active crawl.** Enforced twice: in-process by the existing
  `Allocator` `PolicySticky` lease, and durably by a partial unique index on
  `facebook_crawl_runs(org_id, account_id) WHERE status = 'running' AND
  account_id IS NOT NULL` (§7). DB constraint over
  hopeful application checks.
- **Machine budget unchanged** — `max_active_crawls_per_machine = 1` default from
  PR-C0.5 §4. A 5-account pool does not mean 5 parallel crawls; it means the queue
  drains through whichever single slot the machine budget grants. What the pool
  buys today is **coverage and availability** (an account in cooldown or
  `human_required` doesn't stall the campaign; group-membership coverage spans
  more groups) and a foundation for future distribution across machines. Actual
  cross-account parallelism requires an explicit machine/org budget > 1 later,
  under the PR-C0.5 telemetry-evidence rule — never as a side effect of pool
  size. Same-account parallelism stays forbidden regardless of any budget.
- **Sticky source→account affinity.** A source is preferentially served by the
  account that served it before (group membership lives with the account). Affinity
  is a stable assignment for coverage, never rotation to spread risk.
- **No reassignment on risk stop.** If a run ends `checkpoint_required` /
  `login_required` / `risk_blocked`, its source is **not** instantly re-queued to
  another account. The source waits for the same account to recover (operator path,
  PR-C0.5 §3 invariant) or for an operator to reassign it explicitly. Automatic
  handoff after a wall is the rotation-to-dodge pattern and is forbidden.
- **No retry storm.** A failed/abandoned run is retried at most once
  automatically, after the account's cooldown — and a retry is a **new
  appended run row** (new `run_id`, `attempt + 1`); the old row stays
  immutable history (§10 fencing).

---

## 3. Fresh-lead-only rule

**Rule:** a crawled post becomes a lead **only if** its canonical
`TimestampParse` DTO is eligible under the **confidence-specific freshness gate
in §4**: `exact` compares `posted_at ≥ fresh_cutoff_at`; `derived_relative`
compares `earliest_utc ≥ fresh_cutoff_at` (the whole possible interval must be
fresh — never the representative `posted_at`); `ambiguous`, `unknown`, and
future/invalid timestamps are never eligible. There is **no generic
posted_at-vs-cutoff comparison** anywhere in the pipeline. Everything else may
still be stored as a post (dedup history) but is excluded from lead creation
with a typed reason.

### Server-defined `fresh_cutoff_at`

- Computed **server-side at run dispatch**:
  `fresh_cutoff_at = dispatch_time_utc - campaign.freshness_window` (default 24h).
- Sent to the extension inside the crawl task payload; stored on the run row.
- The extension **never derives the cutoff from its own clock**. Client clocks
  skew and are user-controlled; the cutoff is a server contract so that lead
  eligibility is identical no matter which machine crawled.
- The ingest gate re-checks against the run's stored `fresh_cutoff_at` — the
  extension's early-stop is an optimization, not the authority.

### Typed exclusion reasons (ingest gate)

| Reason code | Meaning |
|---|---|
| `stale_post` | Parsed timestamp is confidently older than `fresh_cutoff_at`. |
| `timestamp_unparsed` | No timestamp found or parse confidence `unknown`. |
| `timestamp_ambiguous` | Parse confidence `ambiguous` and the ambiguity window straddles the cutoff. |
| `timestamp_invalid` | Future or internally inconsistent timestamp (§4) — parser/page anomaly. |
| `duplicate_lead` | A lead already exists for this post identity (§6). |

Counters of each reason ride the existing crawl-progress telemetry (PR-C0.5 §6
extension) so the operator can see *why* a 40-post crawl produced 3 leads. The
operator-facing surfacing of these counters is specified in
[experience.md](../../experiences/fresh-lead-discovery/experience.md).

---

## 4. Timestamp parser confidence model

Facebook renders post age as localized relative text ("2 giờ", "2 hrs", "Hôm
qua", "23 phút"), occasionally as exact datetimes in attributes/tooltips, and
sometimes not at all (interleaved ads, reels blocks). A single "parse to Date"
function would silently guess; the contract instead is:

```text
parsePostTimestamp(node, now_utc) -> TimestampParse {
  posted_at:    string | null,   // best representative ISO timestamp, else null
  confidence:   exact | derived_relative | ambiguous | unknown,
  earliest_utc: string | null,   // OLDEST possible timestamp in the parse interval
  latest_utc:   string | null,   // NEWEST possible timestamp in the parse interval
  // optional typed metadata:
  raw_unit:       minute | hour | day | week | date | none, // typed, never raw text
  parser_version: string,
}
```

`TimestampParse` is **the one canonical DTO** on the whole path: the parser
returns it, the extension wire payload carries it per item verbatim (interval
bounds included), and the server-side freshness gate consumes exactly these
fields. No renamed twins — `posted_at_utc` and `timestamp_confidence` do not
exist anywhere in this design.

Clock authority: the **server** computes `now_utc` and `fresh_cutoff_at` from
the server clock at dispatch and sends both in the task payload; the extension
uses the received `now_utc` for parsing and frontier decisions. The
client/browser clock is **never authoritative** for lead eligibility.

Window semantics: relative units truncate, so "23 giờ" means the post is between
23h and 24h old — `earliest_utc = now - 24h`, `latest_utc = now - 23h`. The
window bounds the *worst case*; eligibility judges the worst case.

| Confidence | Source | Lead eligibility |
|---|---|---|
| `exact` | Machine-readable attribute / permalink datetime. | Eligible **only if** `posted_at ≥ fresh_cutoff_at`. |
| `derived_relative` | Unambiguous relative text ("2 giờ" → post age in [2h, 3h)). | Eligible **only if the entire possible interval is inside the fresh window**: `earliest_utc ≥ fresh_cutoff_at`. The oldest possible interpretation must still be fresh — provably fresh, not plausibly fresh. |
| `ambiguous` | Coarse text ("hôm qua", "1 ngày", "yesterday") — interval too wide or straddles the cutoff. | **Not eligible.** Excluded as `timestamp_ambiguous`. |
| `unknown` | Nothing parseable. | **Not eligible.** Excluded as `timestamp_unparsed`. |

A **future or invalid** timestamp (`posted_at > now_utc`, `earliest_utc >
latest_utc`, or unparseable bounds on an `exact`/`derived_relative` claim) is
**never lead-eligible** — excluded as `timestamp_invalid`; it signals parser or
page anomaly, not freshness.

Worked examples against a 24h cutoff:
- "22 giờ" → window [22h, 23h) old; `earliest_utc` is inside 24h → **eligible**.
- "23 giờ" → window [23h, 24h) old; the worst case touches but does not cross
  the cutoff (`earliest_utc = fresh_cutoff_at`) → **eligible** at the boundary.
- "24 giờ" / "1 ngày" / "hôm qua" → worst case is ≥ 24h old → **not eligible**
  unless an `exact` timestamp proves the post is still inside the window.

Rules:
- The parser is a **pure function** (node text/attrs in, struct out, `now` passed
  in) with table-driven unit tests covering vi/en locales. No DOM walking beyond
  the article node it is handed.
- Only typed fields leave the browser. The raw timestamp text is an implementation
  detail — consistent with the PR-C0.5 privacy rule that raw page text never
  escapes as telemetry.
- A run tracks `parse_confidence_ratio` (share of items with `exact` or
  `derived_relative`). If it drops below a named-constant floor (e.g. 0.5 over ≥10
  items), the run stops with `timestamp_parser_degraded` — a selector-drift alarm,
  not a reason to guess harder.

---

## 5. Temporal frontier stop algorithm

Goal: stop scrolling when the feed has run out of fresh posts, instead of
grinding to `max_items` (the PR-C0 audit measured ~9.6 min of blind scrolling).

Facebook feeds are **not reliably time-sorted**: pinned posts, "popular in group"
re-injections, and async re-ordering mean one stale post proves nothing. The
frontier is therefore a *consecutive-evidence* rule, not a first-stale rule:

```text
stale_streak = 0
for each newly extracted item (post-dedup):
  t = parsePostTimestamp(item, server_now_utc)

  if t.confidence in {exact, derived_relative}:
      if t is fresh per the canonical freshness gate (§4):
          stale_streak = 0
      else:
          stale_streak += 1
  else:
      # ambiguous/unknown RESETS the streak: the streak is consecutive
      # *confident* stale evidence; an unproven item breaks the chain —
      # absence of evidence is not evidence of staleness
      stale_streak = 0

  if stale_streak >= FRONTIER_STALE_STREAK:
      stop(frontier_reached)
```

- `FRONTIER_STALE_STREAK` is a named constant, conservative default (e.g. 8),
  tuned only later under telemetry evidence — same discipline as PR-C0.5 §5
  thresholds.
- The frontier stop **composes with, never overrides**, the existing stops:
  checkpoint/login classifier stop, `no_progress_rounds` cutoff, `max_items`,
  and the parser-degraded stop (§4). First stop wins; each carries its typed
  `exit_reason_code`.
- The frontier never *speeds up* scrolling. It only ends the run earlier. Pacing
  stays whatever PR-C4 (safety track) says it is.
- Stopping at the frontier is recorded as a **successful** run
  (`stopped_safe` + `frontier_reached`), and the source's coverage cursor
  advances (§6) — early exit is the designed outcome, not a failure.

---

## 6. Dedupe strategy

Three layers, each already partially present — this spec assigns each a single
responsibility instead of inventing a new mechanism:

1. **In-run (extension, exists).** `dedupKey` + `seen` Set in `crawl.js` stops
   the same article being emitted twice within one run. Content+author identity
   is the dedup decision (the existing in-code mandate: FB reorders/pins too
   aggressively for timestamp-only logic). Unchanged.
2. **Cross-run post identity (server, exists).** `posts.dedup_hash UNIQUE` +
   insert-or-ignore. A re-seen post is counted as `duplicate_count` telemetry but
   creates no second row. Unchanged.
3. **Lead identity (server, to add).** The fresh-lead gate creates at most one
   lead per `(org_id, post identity)`: a post that stays fresh across two runs
   in the same window must not mint two leads. Enforced by the **additive**
   `facebook_crawl_lead_index` table (§7) — the existing `leads` table is not
   altered. The gate claims identity first, then creates the lead, in one
   transaction: `INSERT INTO facebook_crawl_lead_index ... ON CONFLICT DO
   NOTHING`; a losing insert short-circuits to `duplicate_lead`, a winning
   insert proceeds to lead creation and back-fills `lead_id` on the claimed
   row. Claim-first makes the race between two concurrent ingests a constraint
   decision, not an application-level check.

Coverage cursor: each campaign source keeps `cursor_last_post_at` (same pattern
as `org_crawl_intents.cursor_last_post_at`). A source is **due** when
`now - last_run_at ≥ campaign cadence` — the cursor exists for observability and
frontier sanity checks, not as a correctness gate (dedup layers 2–3 are the
correctness gates, so a cursor reset can never cause duplicate leads).

---

## 7. Data model (PostgreSQL platform plane)

Durable queue/campaign/run/account state is **system of record → PostgreSQL
platform plane**, per `docs/architecture/DATABASE_OWNERSHIP.md` and the PR-C0.5
§7 doctrine. Ephemeral in-run counters stay in the browser/connector.

Table names carry the `facebook_` platform prefix: the platform plane already
holds generic crawl tables (`posts`, `groups`, `jobs`, `org_crawl_intents`),
and this domain is Facebook-specific by design (Taobao/1688 crawling would be
its own vertical, not rows in these tables).

**The authoritative DDL is the shipped schema:
[implementation/postgres-schema.md](./implementation/postgres-schema.md)
(PR-M2B, migrations 0112–0117 as applied by the store boot path).** The draft
DDL formerly inlined in this section had diverged from the shipped schema —
index names (`uq_facebook_crawl_runs_*` vs the shipped `ux_fb_crawl_runs_*`),
the `cancelled` run status, the `task_id` and `duplicate_count` run columns,
and the `platform` column in the lead-index primary key — and was removed in
favor of that single schema authority. The product invariants the schema must
uphold remain owned here:

- **One active crawl per account** — partial unique index on
  `(org_id, account_id) WHERE status = 'running' AND account_id IS NOT NULL`,
  plus a CHECK that a `running` run always has a non-NULL `account_id`.
- **At most one open run per source** (`queued`, `waiting_for_connector_upgrade`,
  and `running` are open states) — no double-queueing a group.
- **At most one retry per abandoned run**, appended as a new row
  (`retry_of_run_id` lineage, `attempt + 1`); run rows are immutable once
  terminal.
- **Org-scoped composite FKs everywhere** — a run cannot reference another
  org's campaign, a source of a different org/campaign, an account outside this
  campaign's pool, or a retry parent from another org.

The partial unique indexes are **safety backstops, not the scheduler**: the
scheduler/lease logic (Allocator lease + Coordinator budget) must still enforce
the invariant *before* dispatch, so a constraint violation is always a logic
bug surfacing, never the normal admission path.

Nullable-column FK semantics: the account-pool FKs on `preferred_account_id`
and `account_id` use the default `MATCH SIMPLE` — while the column is NULL the
FK is not checked, which is exactly the contract: "when non-NULL, must be in
this campaign's pool".

Notes:
- Every table carries `org_id`; every query is org-scoped (hard rule).
- Durable per-account safety state (`cooldown_until`, `last_safe_stop_reason`,
  `recent_automation_window`) is **the PR-C0.5 §7 table, referenced, not
  duplicated** here.
- `org_crawl_intents` is untouched. Campaigns are additive; single-source intents
  keep working. A later, separate decision may migrate intents onto campaigns —
  not assumed by this spec.
- Cookies/session secrets never appear in any of these tables (data-plane law).

---

## 8. Code / file structure

Owned by the implementation layer:
[implementation/code-organization.md](./implementation/code-organization.md)
(runtime ownership, dependency direction, package shape, port contracts, and
transaction seams for PR-M3..PR-M5). Boundary laws recap: pure policy lives in
`internal/services/facebook` (imports no store/server), adapters live in the
composition root, every new file respects the 200-line guard, and reason codes
are centralized — never scattered string literals.

---

## 9. Production impact

**This PR: none.** Docs only.

Future runtime train (§11), impact per the invariants:
- **Additive schema** — new tables plus the org-scoped FK anchor on the existing
  `accounts` table (shipped as migration 0112; index only, no column change).
  No change to `posts`, `leads`, `org_crawl_intents`, or any existing wire/DTO
  contract. Lead-identity dedupe in particular lives in the additive
  `facebook_crawl_lead_index` table (§6, §7), not in an altered `leads` table.
- **Existing crawls unaffected** — the single-intent path keeps its exact
  behavior until a campaign explicitly exists for the org. Campaigns default off.
- **Lead volume becomes intentionally lower** per crawl (stale posts stop minting
  leads). This is the product goal, and the exclusion counters (§3) make the
  delta visible instead of mysterious.
- **No new concurrency** — the machine budget and per-account lease already cap
  parallelism; campaigns only decide *what* runs in the single slot, never *how
  many* slots exist.
- **No auth/session/cookie surface change**; the extension gains one pure parser
  module and consumes two extra task fields (`fresh_cutoff_at`, `now_utc`).

### Connector version compatibility gate

Fresh-lead campaigns are **strict mode**: they require a connector/extension
version that reports the per-item §4 `TimestampParse` DTO — `posted_at`,
`confidence`, and the `earliest_utc`/`latest_utc` interval bounds (PR-M1+).

- Old connectors keep working — they can still submit **legacy crawl results**
  for the existing intent path; nothing breaks for them.
- The campaign scheduler checks the connector's reported capability/version
  **before dispatch**. A strict fresh-lead run is **never dispatched** to a
  connector that cannot prove freshness.
- When every eligible connector for an account is unsupported, the run moves to
  `waiting_for_connector_upgrade` — a **first-class run status** in the §7
  lifecycle (an *open* state: it holds the one-open-run-per-source slot, so the
  source cannot be double-queued while waiting) — surfaced in telemetry/UI. It
  must **not** silently dispatch and produce zero leads, which would be
  indistinguishable from "no fresh posts exist".

---

## 10. Failure handling

Every non-clean end is a typed `exit_reason_code` on the run row; the operator
sees the reason, never a silent stall.

| Failure | Handling |
|---|---|
| Checkpoint / login wall / risk signal | Stop per PR-C0.5; run → `stopped_safe` + reason; account → its safety state; source **stays with the account** (no auto-handoff, §2); `human_required` only clears via the operator path. |
| Connector disconnects mid-run | Server reaper: `running` run with `heartbeat_at` older than a lease timeout → `abandoned` and that row is **immutable from then on**. The retry is a **new appended run row** (new `run_id`, `attempt = old + 1`, `retry_of_run_id = old run_id`), created **once**, after account cooldown — never instant, never a reused/rewritten row. Retry creation is **idempotent**: mark-abandoned + insert-retry run in one transaction with `ON CONFLICT DO NOTHING` on the single-retry unique index (`ux_fb_crawl_runs_one_retry_per_parent`, see [implementation/postgres-schema.md](./implementation/postgres-schema.md)), so two racing reapers produce exactly one retry. |
| Stale worker writes after requeue | `(run_id, attempt)` is the **fencing token**, carried in every dispatch payload. Every heartbeat/progress/finish/lead write is guarded by `WHERE org_id = ? AND id = ? AND attempt = ? AND status = 'running'`. A reaped worker still holds the *old* run_id, whose row is `abandoned` — its writes match **zero rows**, are recorded as `stale_attempt` (telemetry-logged), and mutate nothing. Append-only retries make token reuse structurally impossible: no two dispatches ever share `(run_id, attempt)`. |
| Timestamp parser degrades (selector drift) | Run stops `timestamp_parser_degraded` (§4); campaign keeps serving other sources; alarm surfaces in telemetry. No guessing, no lead creation from unparsed items. |
| Frontier never reached, `max_items` hit | Normal end: `stopped_safe` + `max_items_reached`; cursor advances; next due cycle continues coverage. |
| One account dead, queue non-empty | Scheduler simply never picks it (state not `ready`); other pool accounts drain the rest. A campaign is never blocked by one account. |
| Connector too old for strict mode | Run held in `waiting_for_connector_upgrade` (§9 gate; an open status in the §7 lifecycle); operator sees the reason. Never dispatched to a connector that cannot report the §4 `TimestampParse` DTO. |
| Dispatch/DB race on admission | The two partial unique indexes (§7) make double-admission a constraint violation, not a data corruption; loser retries the scheduler decision. |
| Server restart | Queue and run state are durable; reaper + scheduler recover from the tables alone. No in-memory-only orchestration state. |

Rollback of the whole feature (post-runtime): pause campaigns (`status='paused'`)
— the tables are additive and inert when no campaign is active.

---

## 11. Rollout PR plan

Owned by the implementation layer:
[implementation/rollout.md](./implementation/rollout.md) (PR train M0–M6,
safety-track dependencies, and docs-only rollback). Sequenced to stay releasable
at every step; behavior-changing PRs ship with tests protecting reason codes and
policy decisions.

---

## 12. Acceptance criteria

The runtime train is done when all of these hold, each pinned by a test or a
telemetry assertion:

1. Two dispatch attempts for the same account cannot both reach `running`
   (constraint test on the one-active-per-account partial unique index,
   shipped as `ux_fb_crawl_runs_one_active_account`).
2. A campaign of N sources and a 1-slot machine budget crawls sources strictly
   one at a time, FIFO by priority (scheduler decision unit tests).
3. A post whose worst-case age crosses the cutoff never creates a lead:
   `exact` with `posted_at < fresh_cutoff_at`, and `derived_relative` with
   `earliest_utc < fresh_cutoff_at` (e.g. "24 giờ"), are both excluded as
   `stale_post`; a future/invalid timestamp is excluded as `timestamp_invalid`;
   a `derived_relative` whose whole interval is inside the fresh window
   (e.g. "23 giờ" at a 24h cutoff) is eligible. Boundary cases pinned by
   parser + gate unit tests, both sides consuming the same `TimestampParse` DTO.
4. A post with `ambiguous`/`unknown` timestamp never creates a lead;
   `timestamp_unparsed`/`timestamp_ambiguous` counters reflect it.
5. `fresh_cutoff_at` is set by the server at dispatch and identical in the run
   row, the task payload, and the ingest gate's decision (no client clock input).
6. A feed yielding `FRONTIER_STALE_STREAK` consecutive confidently-stale posts
   stops with `frontier_reached` before `max_items`; an `ambiguous`/`unknown`
   item anywhere in the sequence **resets** the streak and prevents the stop
   (frontier unit tests + run-history assertion).
7. Re-crawling a group within the freshness window creates zero duplicate leads
   (constraint test on `facebook_crawl_lead_index`; two concurrent ingests of
   the same post yield exactly one lead + one `duplicate_lead` exclusion).
8. A run ending in a checkpoint/login state leaves the source un-reassigned and
   the account in the PR-C0.5 state machine; no automatic account handoff occurs.
9. A killed connector yields exactly one `abandoned` row (immutable thereafter)
   plus exactly one **new** run row (`attempt = 2`, new `run_id`,
   `retry_of_run_id` = the abandoned run), created only after cooldown (reaper
   test with injected clock).
10. Parser confidence ratio below the floor stops the run with
    `timestamp_parser_degraded` and creates no leads from unparsed items.
11. All new tables/queries are org-scoped; cross-org access is covered by a test
    in the pattern of `crawl_org_scope_test.go`.
12. No raw timestamp text, page text, or DOM leaves the extension — wire carries
    typed fields only.
13. A strict fresh-lead run facing only pre-PR-M1 connectors is held in
    `waiting_for_connector_upgrade` and never dispatched (scheduler gate test);
    the held run occupies the one-open-run-per-source slot; legacy intent
    crawls on the same connector are unaffected.
14. A heartbeat/progress/finish/lead write carrying a superseded `(run_id,
    attempt)` fencing token — e.g. from a reaped worker whose run was retried
    as a new row — matches zero rows under the `WHERE org_id = ? AND id = ?
    AND attempt = ? AND status = 'running'` guard, mutates nothing, and is
    logged as `stale_attempt` (fencing test with two simulated workers).
15. Two reapers racing over the same abandoned run create exactly one retry
    row (constraint test on the single-retry unique index, shipped as
    `ux_fb_crawl_runs_one_retry_per_parent`; the losing insert is an
    `ON CONFLICT DO NOTHING` no-op).
16. No run row can hold `status = 'running'` with a NULL `account_id`
    (CHECK constraint test); pool membership rows cannot reference an
    account of another org or a nonexistent account (FK test against
    `accounts(org_id, id)`).

---

## 13. Rejected designs

Owned by the decision layer:
[decisions/rejected-designs.md](./decisions/rejected-designs.md) — twelve
rejected alternatives (client-computed cutoff, trusting ambiguous timestamps,
rotation patterns, queue frameworks, and more), each with the reason it must
not come back.
