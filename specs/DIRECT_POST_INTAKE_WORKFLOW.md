# Direct-Post Intake → Comment Continuation

**Status:** ACTIVE (PR-1 data foundation + PR-2 runtime loop landed).
**Track:** Comment Intelligence / Facebook automation. **Aligns with** Architecture
Standard V3 (durable workflow + process manager; no in-memory callback as source of
truth, no `user_context` KV continuation, no generic transactional outbox yet).

## 1. Product behavior

A user prompt like
`Comment bài này cho tôi https://www.facebook.com/groups/ship.viet.my/permalink/4504452536547584/`
is a **direct single-post lead intake + comment** request:

- **Post already a lead** → comment now via the existing `comment_single_post` flow.
- **Post NOT yet a lead** → import exactly this one post, create/upsert it as a normal
  lead, (existing) Telegram lead-notification fires, then plan/queue the comment.
  Future agents see the post as a normal lead.

The P0 transitional response *"Bài viết này chưa có trong hệ thống. Hãy quét/import bài
viết trước khi comment."* is **no longer the desired final behavior** for this prompt
type. The async acknowledgement becomes:
*"Đã nhận bài viết này. Mình sẽ đưa bài viết vào leads của workspace, đọc nội dung và tự
động comment khi đủ dữ liệu."*

This is single-post intake for ONE explicit URL — **not** `scrape_comments`, **not**
bulk crawling.

## 2. PR split

- **PR-1 — data foundation (DONE):** migration `0022_direct_post_comment_workflows`,
  typed coordination store (CRUD + CAS/lease transitions), `GetPostLeadByRef`, status/
  config constants, tests. No runtime behavior change.
- **PR-2 — runtime (DONE):** the narrow `directPostIntake` service (cmd/scraper) that
  `commentSinglePost` calls for an unknown post — it creates/resumes the workflow,
  enqueues ONE `facebook_post` import per `intake_key` (auto-picked connector / worker
  fallback; reused across actors), and returns the async ack (replacing the
  scan-required copy). The DB-polling process manager `runDirectPostIntakeScheduler`
  observes the post lead (`GetPostLeadByRef`) and queues the comment via
  `queueLeadOutreach` exactly once (CAS-guarded), with bounded retry → typed terminal
  failure when the import never materializes. The poller exits cleanly on
  `context.Done()`; leased rows recover via lease expiry. Telegram is NOT sent by the
  workflow (the lead's own creation notification fires once). Copilot owns no
  persistence — it only routes to the cmd/scraper handler.
- **PR-3 — optional hardening:** dedicated worker-binary poller, expiry tuning, a
  job-status oracle (vs. the current bounded-retry-then-import_failed), observability
  for actionable states, and a transactional outbox to harden exactly-once Telegram.

## 3. Idempotency model (TWO keys — deliberately separate)

| Key | Scope | Purpose | Index |
|---|---|---|---|
| `intake_key` | `org_id + canonical_post_url` | one post imported ONCE; the imported lead is shared across requesters | non-unique `(org_id, intake_key)` |
| `idempotency_key` | `org_id + canonical_post_url + acting account + requesting user + action` | one comment-workflow request per actor/action | **UNIQUE** |

Derived in Go (`DirectPostIntakeKey`, `DirectPostIdempotencyKey`) so the semantics stay
centralized; callers never pass raw keys. Multiple users/accounts may each request a
comment on the same post (distinct `idempotency_key`) while the post is imported once
(shared `intake_key`).

## 4. State machine

```
requested ─▶ import_queued ─▶ importing ─▶ lead_created ─▶ comment_queued ─▶ completed
     │                                                          
     └─(retry)─ retry_scheduled ◀── actionable: connector_unavailable | login_required |
                                    challenge_required | import_failed | lead_upsert_failed |
                                    comment_failed ──▶ failed | cancelled
```

CAS transitions only fire from the expected prior status (a stale poller is a clean
no-op, never a clobber). Claimable (poller-advanced) statuses: `requested`,
`import_queued`, `importing`, `lead_created`, `retry_scheduled`.

## 5. Process-manager (PR-2) + constants

A DB poller modeled on `comment_reverify_scheduler` — **durable, crash-safe** because
it reads DB state (the post lead row + the workflow row); no in-memory callback.
`ClaimDueDirectPostCommentWorkflows` leases work (`lease_owner`/`lease_until`) so
multiple poller processes never double-claim (SQLite serializes writers; the loser sees
the future lease and skips).

| Constant | Value | Why |
|---|---|---|
| `DPMaxRetryCount` | 5 | transient connector walls (login/challenge/offline) get several spaced attempts before giving up |
| `DPDefaultLeaseDuration` | 5 min | mirrors the reverify claim lease; a crashed poller's claim is re-offered within 5 min |
| `DPBaseRetryDelay` | 1 min | base backoff for `ScheduleDirectPostRetry` |

## 6. Telegram notification

The imported post becomes a **normal lead**, so the **existing** lead-created
notification path (`leadingest` → `OnLeadCreated` → `control.NotifyLead`, gated by the
`INSERT OR IGNORE` lead insert) fires **once**. The workflow does **not** send a second
Telegram — `telegram_notified` is an *observed* state, not a workflow-owned send.
`telegram_notification_id` is intentionally **omitted** from the schema (nothing would
populate it durably today). A future transactional outbox (Phase E) will harden
exactly-once Telegram delivery; until then the lead-insert-gated notification is the
source.

## 7. Failure / retry semantics

Typed `error_code` + a short `error_message` for operator drill-down. **No secrets**
(cookies/tokens/session) ever in `error_message`. `ScheduleDirectPostRetry` increments
`retry_count`, sets `next_run_at` (exponential backoff `DPBaseRetryDelay<<n`), and
releases the lease; PR-2 stops at `DPMaxRetryCount` (~31-min window) → `failed`.

**Honest terminal reason.** The poller observes only the LEAD (no job-status oracle),
so the terminal `error_code` is `lead_not_observed_after_retries` (`DPErrLeadNotObserved`)
— it does NOT claim a connector/import failure it cannot confirm.

**Re-prompt after failure.** A fresh request for a TERMINAL `failed`/`cancelled`
workflow re-opens it (`ResetDirectPostWorkflowForRetry`: status → `requested`,
`retry_count`→0, error/import_task_id/lease cleared) so the import retries instead of
the ack lying about a dead workflow. `retry_scheduled`/in-progress re-prompts return the
ack and let the poller continue; a different actor whose peer workflow is terminal-failed
gets a NEW workflow + its own import (the failed one's task is not reused).

## 8. Migration & rollback caveat

`0022_direct_post_comment_workflows__sqlite.up.sql` is **additive** and feature-owned.
The repo uses **forward-only** migrations (no `.down.sql` runner), so rollback is a
manual op:

```sql
DROP TABLE direct_post_comment_workflows;
```

- **Before the feature is used:** safe — no data loss.
- **After the feature is used:** this **drops pending workflow state** (in-flight
  imports/continuations are lost). Acceptable because the table is additive and
  feature-owned, but it is **NOT** "no data loss".

## 8a. Identity guard (hotfix — wrong-post protection)

A Facebook GROUP permalink id and a global `story_fbid` can be **different posts**
sharing the same number (`/groups/ship.viet.my/permalink/N` ≠
`permalink.php?story_fbid=N`). A bare `post_fbid` match therefore attached the wrong
post (a Data-Engineer `permalink.php` lead matched a `ship.viet.my` shipping post).

Guard (`GetPostLeadByDirectPostRef` + `FindConflictingPostLead`, used by both the
immediate-comment path and the poller). The bar is **provable identity only**:

- **Match only** (a) the exact canonical `source_url`, OR (b) the same `post_fbid` whose
  lead is in the **same group ref** (`/groups/{ref}/permalink|posts/`, vanity==vanity or
  numeric==numeric).
- **Never match** a bare `post_fbid`, a generic `permalink.php?story_fbid=` lead (no
  group context), a **different named group**, or a **different numeric group**.
- The workflow now carries `group_ref` (populated from the canonical URL).
- **Three-way classification of a same-`post_fbid`, non-matching lead:**
  - generic `permalink.php` (no group) or a **different named** group → a **definite
    conflict**: the poller fails the workflow with
    `error_code = imported_lead_identity_mismatch` and logs SAFE diagnostics
    (requested/canonical/group_ref/post_fbid + observed lead_id/source_url/post_fbid/
    group_fbid — **no secrets**) instead of commenting on the wrong post.
  - a **different numeric** group → **ambiguous, not asserted as a conflict**: it may be
    a Facebook vanity→numeric redirect of the *same* post or an unrelated numeric group
    that merely shares the id — indistinguishable **without import provenance**. The
    poller therefore **retries** (per §7) and lands on the honest
    `lead_not_observed_after_retries`, never a wrong comment and never a false mismatch.
  - no candidate at all → "import pending" → retry (per §7).

**Known limitation (provenance gap):** a legit vanity→numeric *redirect* whose imported
lead is stored under the **numeric** group id (the crawler followed the redirect) does
**not** auto-match — only the exact canonical URL does. It safely degrades to
`lead_not_observed` rather than risking a wrong comment. Closing this needs a
**lead↔import-task provenance link** (the canonical `leads` table carries none today),
so the numeric redirect can be accepted *because it was produced by this workflow's
import* — tracked as PR-3 hardening, not part of this hotfix.

## 8b. Explicit-intake filter override (hotfix — market-signal veto bypass)

A direct-post comment command (`Comment bài này cho tôi <url>`) means the user has
**already chosen** the post as a lead candidate. The generic market-signal filter (the
deterministic scorer, the brain signal gate, and the AI classifier) must therefore **not
veto lead creation** for that one post — otherwise the import analyses the post, rejects
it (`0 qualified leads, 1 rejected by market signal filter`), no lead is created, and the
workflow can never queue the comment (it retries to `lead_not_observed`).

Mechanism (no schema change, no in-memory state):

- The connector crawl-result ingest (`/api/connectors/crawl-result`) recognises an
  explicit intake by the **durable provenance link** `body.TaskID ==
  direct_post_comment_workflows.import_task_id`
  (`FindDirectPostWorkflowByImportTaskID`). Only the post the workflow requested
  (matched by `post_fbid`) is treated as explicit; neighbours keep normal filtering.
- For that post, ingest sets `leadingest.Deps.ForceLead`: every market-signal veto
  (deterministic reject / signal gate / AI reject / cold) is **downgraded to annotation**
  — recorded on the lead as `market_filter_result:<verdict>`,
  `filter_override_applied:true`, `explicit_user_requested:true` — and the lead is
  created/upserted anyway (category floored to `warm`). Normal broad crawls never set
  `ForceLead`, so their filtering is byte-for-byte unchanged.
- **Source-URL preservation:** the connector often reports the lossy
  `permalink.php?story_fbid=N`. Ingest overrides the lead's `source_url`/`post_fbid`/
  `group` to the workflow's **context-preserving canonical** group permalink. This both
  keeps the navigable group URL AND lets the §8a P1.1 exact-canonical lookup match the
  lead (a `permalink.php` lead would otherwise be a §8a *definite conflict*).
- The lead is created through the **normal** ingest path, so the existing lead-created
  Telegram notification fires once, and the poller then observes the lead via the strict
  §8a lookup and queues the comment exactly once. The §8a identity guard is **unchanged**.

This override lives ONLY in the explicit direct-post path. The worker (`facebook_crawl`)
in-process FetchBatch path is not used for Facebook in the current deployment and is not
wired for `ForceLead` — if it is ever used, the same `import_task_id` lookup applies.

## 8c. Zero-trust content/context validation (P1.3 — wrong-content guard)

P1.2's force-lead override was **fail-open**: it could stamp the requested canonical URL
onto an observed item whose post id/context/content was not proven to be the requested
post, and `ForceLead` then bypassed the market veto that was the *only* thing detecting the
mismatch. Incident: a Backend-Jobs post (`author = …(Jobs)`, author profile =
`/groups/1112083256270739/`) was stamped with a `ship.viet.my` URL, force-created as lead
#313, and a jobs-themed comment was posted to a shipping group.

The fix is a shared zero-trust validator (`internal/directpost`) enforcing three layered
invariants, used by **two** independent guards. `ForceLead` may still bypass MARKET-FIT
(cold/relevance) vetoes — it must **never** bypass these:

1. **Identity is positive, never assumed** (`PositivePostIDMatch`, fail-CLOSED): the
   observed post id must be present and equal the requested id (or the source URL must
   positively canonicalize to the requested canonical when the requested id is unknown).
   An absent observed id is never assumed to be the requested post.
2. **Context must not conflict** (`ContextConflict`): an author profile that is a
   `/groups/{other}/` URL (a real post author is a user, so a group author = foreign-context
   grab), or a different **named** source/group, is a conflict. A different **numeric**
   group is left ambiguous (possible vanity→numeric redirect) to preserve valid P1.2 cases.
3. **Content must be meaningful** (`ValidContent`): after stripping FB UI-chrome tokens and
   collapsing repetition, near-empty / boilerplate extractions are rejected. The floor is
   low so short-but-real posts pass.

**P1.3A — ingest force-gate** (`internal/server/agent/crawl_direct_post.go`,
`validateDirectPostObservedItem`): only a `Valid` item is force-created with the canonical
URL stamped. A `IdentityMatched` but invalid item (the requested post came back poisoned)
fails the workflow with `imported_item_context_mismatch` / `lead_content_invalid` and
creates **no** lead. A non-matching neighbour falls through to normal filtering.

**P1.3B — pre-comment gate** (`cmd/scraper/direct_post_guard.go`,
`directPostLeadTargetMismatch`): even a strict-canonical-matched lead (P1.1) is re-validated
before queueing; a poisoned lead already in the DB (incl. pre-fix rows) fails the workflow
with `lead_target_context_mismatch` and queues **no** comment.

Account note: the single-post import is deliberately not pinned to the requester's account
(it only *reads*; `submitOpenCrawl` auto-picks any ready connector), while the comment uses
the workflow's account — so an import on a different account (#50) than the comment (#49) is
**by design**, not a routing bug. That an import account may not be a group member (→ FB
serves a wrong/unavailable post) is a plausible contributor to wrong extraction, but the
P1.3A/B validation catches the wrong content regardless of which account scraped it.

### P1.3C — send-time visual verification (NEXT REQUIRED PR, not in this hotfix)

Backend containment (A+B) cannot see what the browser renders. P1.3C must add a **pre-submit**
check in the connector/executor: before clicking Send, verify the *currently visible* target
(URL / post id / group / page / author) matches the expected target; on mismatch or unknown,
**abort before Send**, keep the tab open for inspection, and report a typed reason
(`context_mismatch` / `human_required`). It must never submit first and label it
`submitted_unverified`. No existing lightweight pre-submit backend hook can do this without a
browser/extension change, so it is deferred to its own PR. Operational pause until then:
`systemctl stop thg-worker.service`.

## 9. Ownership & boundaries

- `direct_post_comment_workflows` is owned by `internal/store/coordination` (process-
  manager runtime substrate). Single-table CRUD — it imports **no** leads/outbound.
- `GetPostLeadByRef` lives in `internal/store/leads` (post-only lookup; excludes
  commenter leads that share the post ref).
- PR-2: Copilot initiates via a **narrow intake port** (injected at the composition
  root); Copilot does NOT write lead/outbound/telegram tables. Comment planning stays
  in the outbound/`queueLeadOutreach` owner; lead creation stays in `leadingest`.
