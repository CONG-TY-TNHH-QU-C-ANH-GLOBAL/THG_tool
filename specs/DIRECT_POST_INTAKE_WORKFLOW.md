# Direct-Post Intake → Comment Continuation

**Status:** ACTIVE (PR-1 data foundation landed; PR-2 runtime pending).
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

- **PR-1 — data foundation (this PR):** migration `0022_direct_post_comment_workflows`,
  typed coordination store (CRUD + CAS/lease transitions), `GetPostLeadByRef`, status/
  config constants, tests. **No runtime behavior change** — nothing reads/writes the
  table except tests.
- **PR-2 — runtime:** narrow intake service + Copilot unknown→intake (replaces the
  scan-required copy with the async ack) + the DB-polling process manager
  (`runDirectPostIntakeScheduler`: observe post lead → queue comment, idempotent,
  failure/actionable states) + full A–F test matrix.
- **PR-3 — optional hardening:** worker-path coverage, expiry/retry tuning, observability.

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

Typed `error_code` (e.g. `login_required`, `import_failed`) + a short `error_message`
for operator drill-down. **No secrets** (cookies/tokens/session) ever in
`error_message`. `ScheduleDirectPostRetry` increments `retry_count`, sets `next_run_at`,
and releases the lease; PR-2 stops retrying at `DPMaxRetryCount` → `failed`.

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

## 9. Ownership & boundaries

- `direct_post_comment_workflows` is owned by `internal/store/coordination` (process-
  manager runtime substrate). Single-table CRUD — it imports **no** leads/outbound.
- `GetPostLeadByRef` lives in `internal/store/leads` (post-only lookup; excludes
  commenter leads that share the post ref).
- PR-2: Copilot initiates via a **narrow intake port** (injected at the composition
  root); Copilot does NOT write lead/outbound/telegram tables. Comment planning stays
  in the outbound/`queueLeadOutreach` owner; lead creation stays in `leadingest`.
