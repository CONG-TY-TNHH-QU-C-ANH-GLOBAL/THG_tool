# Facebook Multi-Group Fresh-Lead Crawl Orchestration Spec (PR-M0)

Track: **Facebook Automation Reliability**. Type: **architecture / code-spec baseline.**
Status: **draft — docs only, no runtime change.** Companion to
[CRAWLER_ACCOUNT_SAFETY_SPEC.md](CRAWLER_ACCOUNT_SAFETY_SPEC.md) (PR-C0.5) and
[CRAWL_SPEED_CHECKPOINT_AUDIT.md](CRAWL_SPEED_CHECKPOINT_AUDIT.md) (PR-C0).

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

An org wants "leads from the last 24 hours across my 20 groups, every few hours".
Today the system can only express that as 20 independent `org_crawl_intents`, each
crawling until `max_items` regardless of post age, each dispatched with no shared
plan. Consequences:

- **Stale waste.** A group whose feed surfaces week-old posts burns the whole
  `max_items` budget on posts that will never become leads. Crawl time is the
  scarce, risk-carrying resource; spending it on stale content is pure loss.
- **No freshness contract.** `posted_at` is empty on the wire, so "only fresh posts
  become leads" cannot be enforced anywhere — not in the extension, not at ingest.
- **No cross-group orchestration.** Nothing sequences 20 groups over a pool of 5
  accounts under the machine budget; nothing records which group was covered when,
  or which account is mid-crawl.
- **Duplicate leads across runs.** Re-crawling a group re-sees recent posts; only
  the `dedup_hash` insert guard stands between a re-seen post and a duplicate lead.

The answer is a **campaign**: a durable, org-scoped plan (groups + freshness window
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
fresh-lead gate at ingest (server): posted_at ≥ fresh_cutoff_at AND confidence OK
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
  account_id > 0` (§7). DB constraint over
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
- **No retry storm.** A failed run re-enters `queued` at most once automatically,
  after the account's cooldown, with the attempt counter carried on the run row.

---

## 3. Fresh-lead-only rule

**Rule:** a crawled post becomes a lead **only if** its parsed
`posted_at ≥ fresh_cutoff_at` **and** the timestamp parse confidence is
trustworthy (§4). Everything else may still be stored as a post (dedup history)
but is excluded from lead creation with a typed reason.

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
| `duplicate_lead` | A lead already exists for this post identity (§6). |

Counters of each reason ride the existing crawl-progress telemetry (PR-C0.5 §6
extension) so the operator can see *why* a 40-post crawl produced 3 leads.

---

## 4. Timestamp parser confidence model

Facebook renders post age as localized relative text ("2 giờ", "2 hrs", "Hôm
qua", "23 phút"), occasionally as exact datetimes in attributes/tooltips, and
sometimes not at all (interleaved ads, reels blocks). A single "parse to Date"
function would silently guess; the contract instead is:

```text
parsePostTimestamp(node, now_utc) -> {
  posted_at_utc: string | null,   // ISO, null when unknown
  earliest_utc:  string | null,   // OLDEST possible timestamp in the parse window
  latest_utc:    string | null,   // NEWEST possible timestamp in the parse window
  confidence:    exact | derived_relative | ambiguous | unknown,
  raw_unit:      minute | hour | day | week | date | none,  // typed, never raw text
}
```

Window semantics: relative units truncate, so "23 giờ" means the post is between
23h and 24h old — `earliest_utc = now - 24h`, `latest_utc = now - 23h`. The
window bounds the *worst case*; eligibility judges the worst case.

| Confidence | Source | Lead eligibility |
|---|---|---|
| `exact` | Machine-readable attribute / permalink datetime. | Eligible if `posted_at_utc ≥ fresh_cutoff_at`. |
| `derived_relative` | Unambiguous relative text ("2 giờ" → post age in [2h, 3h)). | Eligible **only if the entire possible window is inside the fresh window**: `earliest_utc ≥ fresh_cutoff_at`. The oldest possible interpretation must still be fresh — provably fresh, not plausibly fresh. |
| `ambiguous` | Coarse text ("hôm qua", "1 ngày", "yesterday") — window too wide or straddles the cutoff. | **Not eligible.** Excluded as `timestamp_ambiguous`. |
| `unknown` | Nothing parseable. | **Not eligible.** Excluded as `timestamp_unparsed`. |

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
  t = parsePostTimestamp(item, now)
  if t.confidence in {exact, derived_relative}:
      if t is fresh (per §4 eligibility):  stale_streak = 0
      else:                                stale_streak += 1
  # ambiguous/unknown items DO NOT advance the streak — absence of
  # evidence is not evidence of staleness
  if stale_streak >= FRONTIER_STALE_STREAK:  stop(frontier_reached)
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
   in the same window must not mint two leads. Enforced by a unique constraint at
   lead ingest keyed on the post's dedup identity, surfaced as `duplicate_lead`.

Coverage cursor: each campaign source keeps `cursor_last_post_at` (same pattern
as `org_crawl_intents.cursor_last_post_at`). A source is **due** when
`now - last_run_at ≥ campaign cadence` — the cursor exists for observability and
frontier sanity checks, not as a correctness gate (dedup layers 2–3 are the
correctness gates, so a cursor reset can never cause duplicate leads).

---

## 7. Proposed data model (PostgreSQL platform plane)

Durable queue/campaign/run/account state is **system of record → PostgreSQL
platform plane**, per `docs/architecture/DATABASE_OWNERSHIP.md` and the PR-C0.5
§7 doctrine. Ephemeral in-run counters stay in the browser/connector. The
migration ships as its **own RED-reviewed PR** (PR-M2), never smuggled in.

Table names carry the `facebook_` platform prefix: the platform plane already
holds generic crawl tables (`posts`, `groups`, `jobs`, `org_crawl_intents`),
and this domain is Facebook-specific by design (Taobao/1688 crawling would be
its own vertical, not rows in these tables).

```sql
-- The plan
CREATE TABLE facebook_crawl_campaigns (
    id                       BIGSERIAL PRIMARY KEY,
    org_id                   BIGINT NOT NULL,
    name                     TEXT NOT NULL,
    freshness_window_minutes INTEGER NOT NULL DEFAULT 1440,   -- 24h
    cadence_minutes          INTEGER NOT NULL DEFAULT 240,
    max_items_per_run        INTEGER NOT NULL DEFAULT 50,
    status                   TEXT NOT NULL DEFAULT 'active',  -- active|paused|archived
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Which groups, with per-source cursor + affinity
CREATE TABLE facebook_crawl_campaign_sources (
    id                 BIGSERIAL PRIMARY KEY,
    campaign_id        BIGINT NOT NULL REFERENCES facebook_crawl_campaigns(id),
    org_id             BIGINT NOT NULL,
    source_url         TEXT NOT NULL,
    source_label       TEXT NOT NULL DEFAULT '',
    priority           INTEGER NOT NULL DEFAULT 0,
    preferred_account_id BIGINT NOT NULL DEFAULT 0,           -- sticky affinity
    cursor_last_post_at TIMESTAMPTZ,
    last_run_at        TIMESTAMPTZ,
    status             TEXT NOT NULL DEFAULT 'active',
    UNIQUE (campaign_id, source_url)
);

-- Which accounts may serve the campaign
CREATE TABLE facebook_crawl_campaign_accounts (
    campaign_id BIGINT NOT NULL REFERENCES facebook_crawl_campaigns(id),
    org_id      BIGINT NOT NULL,
    account_id  BIGINT NOT NULL,
    PRIMARY KEY (campaign_id, account_id)
);

-- The queue + append-only run history (one table, status is the queue)
CREATE TABLE facebook_crawl_runs (
    id               BIGSERIAL PRIMARY KEY,
    campaign_id      BIGINT NOT NULL REFERENCES facebook_crawl_campaigns(id),
    source_id        BIGINT NOT NULL REFERENCES facebook_crawl_campaign_sources(id),
    org_id           BIGINT NOT NULL,
    account_id       BIGINT NOT NULL DEFAULT 0,               -- 0 until admitted
    status           TEXT NOT NULL DEFAULT 'queued',
        -- queued|running|succeeded|stopped_safe|failed|abandoned
    exit_reason_code TEXT NOT NULL DEFAULT '',
    fresh_cutoff_at  TIMESTAMPTZ,                             -- set at dispatch
    attempt          INTEGER NOT NULL DEFAULT 1,
    posts_seen       INTEGER NOT NULL DEFAULT 0,
    fresh_lead_count INTEGER NOT NULL DEFAULT 0,
    stale_skipped    INTEGER NOT NULL DEFAULT 0,
    unparsed_count   INTEGER NOT NULL DEFAULT 0,
    queued_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at       TIMESTAMPTZ,
    heartbeat_at     TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);

-- THE invariant: 1 account = max 1 active crawl. Org-scoped; excludes the
-- account_id=0 "not yet admitted" sentinel so queued rows never collide.
CREATE UNIQUE INDEX uq_facebook_crawl_runs_one_active_per_org_account
    ON facebook_crawl_runs(org_id, account_id)
    WHERE status = 'running' AND account_id > 0;
-- and: at most 1 queued/running run per source (no double-queueing a group)
CREATE UNIQUE INDEX uq_facebook_crawl_runs_one_open_per_org_source
    ON facebook_crawl_runs(org_id, source_id)
    WHERE status IN ('queued','running');
```

The partial unique indexes are **safety backstops, not the scheduler**: the
scheduler/lease logic (Allocator lease + Coordinator budget) must still enforce
the invariant *before* dispatch, so a constraint violation is always a logic
bug surfacing, never the normal admission path.

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

Respecting the boundary laws (`internal/services/facebook` imports no store/server;
adapters live in the composition root) and the 200-line file guard:

```text
internal/services/facebook/freshlead/
    freshness.go        # §3 eligibility: pure fn (parsed ts, cutoff) -> eligible|reason
    frontier.go         # §5 streak algorithm: pure, state in -> decision out
    schedule.go         # §2 scheduler decision: (queue, accounts, budgets) -> pick|none
    reasons.go          # typed exit/exclusion reason codes (single source)
    *_test.go           # table-driven; no DB, no browser, no clock beyond passed-in now

internal/store/crawl/
    campaigns.go        # campaign + sources + accounts CRUD (org-scoped, dialect-aware)
    runs.go             # queue ops: enqueue, admit (partial-index guarded), finish, reap

cmd/scraper/            # composition root: wires store <-> freshlead policy <-> jobs
    (adapter files; no policy logic here)

local-connector-extension/platforms/facebook/       # post-PR-C2.5 topology:
    crawl_time.js           # §4 DOM timestamp extraction + confidence model (pure)
    crawl_freshness.js      # §3 eligibility + §5 frontier streak policy (pure)
    crawl_time.test.mjs
    crawl_freshness.test.mjs

local-connector-extension/content/crawl.js
    # remains the orchestrator/wiring layer ONLY: consumes fresh_cutoff_at from
    # the task payload, feeds items through crawl_freshness, emits per-item
    # posted_at/confidence. DOM timestamp extraction and freshness policy live
    # under platforms/facebook/ (beside crawl_pacing.js, crawl_progress.js,
    # crawl_post_identity.js) — never inline in crawl.js.
```

Every new file ≤200 lines; policy functions pure; reason codes centralized in one
`reasons.go`, not scattered string literals.

---

## 9. Production impact

**This PR: none.** Docs only.

Future runtime train (§11), impact per the invariants:
- **Additive schema** — new tables only; no change to `posts`, `leads`,
  `org_crawl_intents`, or any existing wire/DTO contract.
- **Existing crawls unaffected** — the single-intent path keeps its exact
  behavior until a campaign explicitly exists for the org. Campaigns default off.
- **Lead volume becomes intentionally lower** per crawl (stale posts stop minting
  leads). This is the product goal, and the exclusion counters (§3) make the
  delta visible instead of mysterious.
- **No new concurrency** — the machine budget and per-account lease already cap
  parallelism; campaigns only decide *what* runs in the single slot, never *how
  many* slots exist.
- **No auth/session/cookie surface change**; the extension gains one pure parser
  module and consumes one extra task field.

### Connector version compatibility gate

Fresh-lead campaigns are **strict mode**: they require a connector/extension
version that reports per-item `posted_at` + `timestamp_confidence` (PR-M1+).

- Old connectors keep working — they can still submit **legacy crawl results**
  for the existing intent path; nothing breaks for them.
- The campaign scheduler checks the connector's reported capability/version
  **before dispatch**. A strict fresh-lead run is **never dispatched** to a
  connector that cannot prove freshness.
- When every eligible connector for an account is unsupported, the run stays in
  a visible `waiting_for_connector_upgrade` state (typed reason on the run row,
  surfaced in telemetry/UI) — it must **not** silently dispatch and produce
  zero leads, which would be indistinguishable from "no fresh posts exist".

---

## 10. Failure handling

Every non-clean end is a typed `exit_reason_code` on the run row; the operator
sees the reason, never a silent stall.

| Failure | Handling |
|---|---|
| Checkpoint / login wall / risk signal | Stop per PR-C0.5; run → `stopped_safe` + reason; account → its safety state; source **stays with the account** (no auto-handoff, §2); `human_required` only clears via the operator path. |
| Connector disconnects mid-run | Server reaper: `running` run with `heartbeat_at` older than a lease timeout → `abandoned`; re-queued **once** (attempt=2) after account cooldown. Never instant. |
| Timestamp parser degrades (selector drift) | Run stops `timestamp_parser_degraded` (§4); campaign keeps serving other sources; alarm surfaces in telemetry. No guessing, no lead creation from unparsed items. |
| Frontier never reached, `max_items` hit | Normal end: `stopped_safe` + `max_items_reached`; cursor advances; next due cycle continues coverage. |
| One account dead, queue non-empty | Scheduler simply never picks it (state not `ready`); other pool accounts drain the rest. A campaign is never blocked by one account. |
| Connector too old for strict mode | Run held in `waiting_for_connector_upgrade` (§9 gate); operator sees the reason. Never dispatched to a connector that cannot report `posted_at`/`timestamp_confidence`. |
| Dispatch/DB race on admission | The two partial unique indexes (§7) make double-admission a constraint violation, not a data corruption; loser retries the scheduler decision. |
| Server restart | Queue and run state are durable; reaper + scheduler recover from the tables alone. No in-memory-only orchestration state. |

Rollback of the whole feature (post-runtime): pause campaigns (`status='paused'`)
— the tables are additive and inert when no campaign is active.

---

## 11. Rollout PR plan

One branch/PR each; behavior-changing PRs ship with tests protecting reason codes
and policy decisions. Sequenced to stay releasable at every step.

| PR | Scope | Behavior change? |
|---|---|---|
| **PR-M0** | This spec + registry entry. | Docs only. |
| **PR-M1** | Extension timestamp parser (`platforms/facebook/crawl_time.js` + `crawl_time.test.mjs`, pure + tested) and per-item `posted_at`/`confidence` on the existing crawl wire; `content/crawl.js` gains wiring only. | Additive telemetry; fills the currently-empty `posted_at`. No stop-logic change. |
| **PR-M2** | Platform migration: §7 tables + partial unique indexes. **RED — own reviewed PR.** | Schema only; nothing reads it yet. |
| **PR-M3** | Pure policy package `freshlead` (freshness gate, frontier, scheduler decision, reason codes) + store CRUD. | No wiring; dead code with tests. |
| **PR-M4** | Scheduler wiring in composition root: campaign → queue → admit via Allocator lease + machine budget + DB constraint. | New orchestration path; existing intent path untouched. |
| **PR-M5** | Crawl task carries `fresh_cutoff_at`; crawl loop consumes frontier decision; ingest applies the fresh-lead gate + lead-identity dedup. | The fresh-lead-only behavior lands here, telemetry-visible. |
| **PR-M6** | Operator UI: campaign CRUD, run history, exclusion counters. | UI only. |

Dependency on the safety track: PR-M4/M5 assume PR-C2 (classifier stop) and
PR-C3 (Coordinator) are in place or land together — the campaign scheduler calls
the Coordinator, it does not reimplement it.

---

## 12. Acceptance criteria

The runtime train is done when all of these hold, each pinned by a test or a
telemetry assertion:

1. Two dispatch attempts for the same account cannot both reach `running`
   (constraint test on `uq_facebook_crawl_runs_one_active_per_org_account`).
2. A campaign of N sources and a 1-slot machine budget crawls sources strictly
   one at a time, FIFO by priority (scheduler decision unit tests).
3. A post whose worst-case age crosses the cutoff never creates a lead:
   `exact` with `posted_at_utc < fresh_cutoff_at`, and `derived_relative` with
   `earliest_utc < fresh_cutoff_at` (e.g. "24 giờ"), are both excluded as
   `stale_post`; a `derived_relative` whose whole window is inside the fresh
   window (e.g. "23 giờ" at a 24h cutoff) is eligible. Boundary cases pinned by
   parser + gate unit tests.
4. A post with `ambiguous`/`unknown` timestamp never creates a lead;
   `timestamp_unparsed`/`timestamp_ambiguous` counters reflect it.
5. `fresh_cutoff_at` is set by the server at dispatch and identical in the run
   row, the task payload, and the ingest gate's decision (no client clock input).
6. A feed yielding `FRONTIER_STALE_STREAK` consecutive confidently-stale posts
   stops with `frontier_reached` before `max_items` (frontier unit tests +
   run-history assertion).
7. Re-crawling a group within the freshness window creates zero duplicate leads
   (lead-identity constraint test).
8. A run ending in a checkpoint/login state leaves the source un-reassigned and
   the account in the PR-C0.5 state machine; no automatic account handoff occurs.
9. A killed connector yields exactly one `abandoned` → re-queue (attempt 2), only
   after cooldown (reaper test with injected clock).
10. Parser confidence ratio below the floor stops the run with
    `timestamp_parser_degraded` and creates no leads from unparsed items.
11. All new tables/queries are org-scoped; cross-org access is covered by a test
    in the pattern of `crawl_org_scope_test.go`.
12. No raw timestamp text, page text, or DOM leaves the extension — wire carries
    typed fields only.
13. A strict fresh-lead run facing only pre-PR-M1 connectors is held in
    `waiting_for_connector_upgrade` and never dispatched (scheduler gate test);
    legacy intent crawls on the same connector are unaffected.

---

## 13. Rejected designs

| Design | Why rejected |
|---|---|
| **Client-computed freshness cutoff** (extension subtracts 24h from its own clock) | Client clocks skew and are user-controlled; lead eligibility would differ per machine. Cutoff is a server contract (§3). |
| **Trusting ambiguous timestamps** ("hôm qua" counts as fresh) | Fresh-lead-only means provably fresh; plausible-but-unproven posts pollute the lead queue with stale contacts. Excluded with a typed reason instead. |
| **Freshest-interpretation-wins at the margin** (`derived_relative` eligible when `latest_utc ≥ fresh_cutoff_at`) | Admits posts whose oldest possible age is already past the cutoff ("24 giờ" would pass) — plausibly fresh, not provably fresh. Replaced by the strict whole-window rule `earliest_utc ≥ fresh_cutoff_at` (§4). |
| **Timestamp-based dedup** (replace content dedup with posted_at identity) | Already rejected in `crawl.js` (in-code mandate): FB reorders/pins/async-injects too aggressively; timestamps are also the *least* reliable extracted field. |
| **First-stale-post stop** (stop the moment one old post appears) | Pinned/re-injected posts make single-post evidence worthless; would truncate runs at the first pin. Consecutive-streak frontier instead (§5). |
| **Round-robin account rotation per source** (spread each group over many accounts for throughput) | Rotation is the coordinated-inauthentic-behaviour pattern; also destroys the membership/affinity model. Sticky affinity + single machine slot instead (§2). |
| **Auto-handoff of a source to the next account after a checkpoint** | Rotation-to-dodge; explicitly forbidden by PR-C0.5. Source waits for operator/recovery. |
| **Relying on Facebook's "sort by new" feed parameter as the freshness guarantee** | FB changes/ignores the parameter unpredictably; it may be used as a *hint* in the task URL but never replaces per-post timestamp proof or the frontier. |
| **Shipping raw page HTML server-side to parse timestamps centrally** | Violates the privacy rule (no full-page DOM off the browser); parser runs client-side, typed fields only. |
| **A separate queue service / generic scheduler framework** | A status column + two partial unique indexes on `facebook_crawl_runs` *is* the queue; no second use case justifies a framework (PR-C0.5 §8 discipline). |
| **Extending `org_crawl_intents` in place** (add campaign columns to the intents table) | Conflates two lifecycles (a personal recurring intent vs an org campaign plan with pool + runs); would force RED changes to a live table for a new feature. Additive tables instead (§7). |
| **Auto-clearing `human_required` after a timer to keep the campaign moving** | Forbidden by the PR-C0.5 invariant; a campaign's throughput never overrides account safety. |

---

## Rollback

Docs-only spec. Rollback = revert this file + its `SPEC_REGISTRY.json` entry. No
runtime, schema, contract, or data-plane surface is touched by this PR.
