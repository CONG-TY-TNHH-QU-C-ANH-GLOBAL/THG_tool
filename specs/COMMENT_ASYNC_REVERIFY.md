# Comment Verification: Forensics, Soft Touch & Async Reverify

**Track:** Facebook Automation Reliability / Comment Intelligence.
**Status:** PR-1 Parts A–C shipped (forensics, soft-touch semantics, UI copy). Part D
(async reverify) is **DESIGN — not yet built**.

## Problem

Comments that genuinely post in a Facebook GROUP frequently fail the in-window DOM
proof (lazy render, "Most relevant" sort, async comment nodes). They land as
`optimistic_success` → ledger `submitted_unverified` ("Đã gửi, đang chờ xác minh"),
or as `shadow_rejected` ("Thất bại"). Two consequences:

1. The lead reads as *untouched* (only `succeeded` is a verified touch), so the
   planner could comment again → real duplicate on Facebook.
2. The operator can't tell a verify false-negative from a true failure.

`feedback_verified_state_centric` forbids promoting `submitted_unverified` to
`succeeded` on a guess. So we (a) treat it as a **soft touch** that holds the lead,
and (b) later **reverify out-of-band** and, only on real proof, append a correction.

## Shipped in PR-1

### Part A — Forensics (done)
- `models.CommentForensicsRow` + `models.ClassifyCommentForensics` (pure): buckets an
  attempt into `failed_before_submit` / `submitted_unverified` /
  `likely_verify_false_negative` / `real_failed` / `redirected_or_context_drift` /
  `verified`. Submit/verify booleans are DERIVED from the terminal `ExecutionOutcome`
  (`SubmitReachedForOutcome`) because the raw extension booleans were not persisted.
- `coordination.CommentForensicsByTargetURLs(ctx, orgID, urls)`: joins the latest
  outbound + latest execution_attempt + action_ledger, parses `evidence_json`
  (comment_permalink, page_url_after, nav_diagnostic{phase, redirect_class},
  screenshot_path, notes).
- Endpoint: `GET|POST /api/superadmin/comment-forensics?org_id=&urls=` (founder).

### Part B — Soft-touch semantics (done)
- `models.IsLedgerOutcomeHardVerifiedTouch` (only `succeeded`/`dom_verified`),
  `IsLedgerOutcomeSoftTouch` (`submitted_unverified`/`optimistic_success` **with**
  submit accepted), `IsRetryableBeforeSubmitFailure` (target_not_reached, composer*,
  …).
- New lifecycle state `waiting_verification` + next_action `verify_later`. A lead with
  a soft touch newer than `LeadLifecyclePolicy.VerificationCooldown` (default 30m,
  `LEAD_VERIFICATION_COOLDOWN_MIN`) is held there; a later hard touch / customer reply
  wins. After the cooldown it falls back to a normal retry-eligible state.
- The work queue (and therefore the planner via `WorkQueueLeads`) **excludes**
  `waiting_verification` by default → blocks immediate re-comment. It surfaces only
  when a caller explicitly requests the state (reverify/retry).
- A failure BEFORE submit is NOT a soft touch → the lead stays eligible.

### Part C — UI copy (done)
- "Đã gửi nhưng chưa xác minh" → **"Đã gửi, đang chờ xác minh"**.
- `commentActions()` exposes **"Mở post"** (open target) always, and **"Xác minh lại"**
  for the unverified state — currently `enabled:false` with a TODO pointing here until
  Part D ships.

## Part D — Async reverify (DESIGN)

Goal: out-of-band, append-only upgrade of a soft touch to a verified touch when — and
only when — the comment is actually found on the post.

### Trigger & schedule
- A new periodic worker (mirror `runAutoArchiveScheduler`) OR an extension job.
- For each `submitted_unverified` ledger row with no terminal reverify yet, schedule a
  reverify **2–5 minutes** after `performed_at` (configurable
  `COMMENT_REVERIFY_DELAY_MIN`, default 3) — long enough for lazy render + moderation
  to settle, short enough that the verification cooldown (30m) still covers it.

### Reverify step (extension-driven; observable)
1. Open the target post (the existing nav + identity gates).
2. Load comments (expand "Xem thêm bình luận"; switch to "Mới nhất" if available).
3. Search for a comment whose author FBUID == the executing account's `c_user` AND
   whose text fuzzy-matches the normalized queued content.
4. Report `found` + `comment_permalink` (the same proof contract).

### Correction (append-only — never mutate the old row)
- **Found** → append a NEW `action_ledger` row (or a typed engagement-correction event,
  per `feedback_append_only_correction_events`) with outcome `succeeded` and reason
  `reverified`, linked to the same outbound. The original `submitted_unverified` row
  stays. The engagement projection (`outcome='succeeded'`) now counts the lead as a
  hard touch; lifecycle leaves `waiting_verification`.
- **Not found after the reverify budget** → append a `shadow_rejected`/`failed`
  correction (the soft touch really didn't land) so the lead becomes retry-eligible.
- Record on the attempt/outbound: `reverify_attempted_at`, `reverify_outcome`
  (`verified` | `not_found` | `error`), `reverify_reason`. Add via migration
  (nullable columns; never backfilled destructively).

### Invariants
- Never UPDATE a historical ledger row (append-only; CI gate §6.4).
- Never promote on a guess — only on a positive author+text match.
- Reverify is idempotent: re-running a verified row is a no-op (the `succeeded`
  correction already exists).
- Tenant-scoped; emit a typed `events.*` metric (org_id, scanned, reverified,
  not_found, duration_ms) like the archive sweep.

### Tests (Part D, when built)
- soft touch + comment present at reverify → appends `succeeded` correction; lead
  becomes a hard touch; original row untouched.
- soft touch + comment absent → appends failure correction; lead retry-eligible.
- reverify idempotent on an already-verified outbound.
- reverify never mutates an existing ledger row (row count grows, old row unchanged).
