# Facebook Crawl Speed & Checkpoint Audit (PR-C0)

Track: **Facebook Automation Reliability**. Type: **audit-only, no behavior change**.
Branch: `audit/facebook-crawl-speed-checkpoint-prc0` (from `origin/main` @ `8fd43a2`,
post Reel PR-R3 merge). Trigger case: `facebook_crawl` task `autocrawl-27-495423`,
org #5, account #50, group `1312868109620530` (CHRONOLOGICAL). Observed: `0/50` at
22:47 → `1/50` → stuck → `5/50` at 22:51 → stuck to 23:07 → `10/50` at 23:08.

This is a reliability/observability audit. **Not** a checkpoint-bypass, stealth,
evasion, or speed-increase sprint. No stealth/challenge-solving/account-rotation is
proposed anywhere below.

---

## 1. Which crawl path actually ran

There are **two independent crawlers** in the codebase. The production log came from
the **extension path**, not the Go CDP path.

| | Go CDP path | Extension path (this incident) |
|---|---|---|
| Entry | `facebookcrawl.Handler.Handle` → `rt.FetchBatch` | connector command `type:"crawl"` |
| Runtime | `runtime.CDPRuntime.FetchBatch` (Docker Chrome) | `THGContentCrawl.crawlVisibleFacebookPosts` |
| Scrolls? | **No** — single `extractPostsJS`, `offset>0`→nil | **Yes** — up to 150 scroll passes |
| Progress | job % (`UpdateProgress`) | `thg_crawl_progress` heartbeats `fetched/max` |
| Checkpoint | `ErrFacebookCheckpoint` → `human_required` | classifiers exist (proof/state/outbox) but **not consumed by the live crawl loop** |

Proof the incident is the extension path: the user-visible message
`Stage: scraping. Progress: 5/50 posts` is produced by
`system.NotifyCrawlProgress` from the `fetched/max` heartbeat, which only the
extension emits (`emitProgress` → `thg_crawl_progress`). The Go path reports a
percentage, never `X/N`.

### Flow map (extension path)

```text
recurring intent (org_crawl_intents) ──dispatch──▶ connector command {type:"crawl", payload.task}
  └▶ content/bridge.js  thgExecuteCommand()            [content/bridge.js:29-56]
       └▶ THGContentCrawl.crawlVisibleFacebookPosts(task, expectedUrl, accountId)
            (local-connector-extension/content/crawl.js:523-721)
            ├─ locationMatchesExpected(expectedUrl)  → wrong_page guard   [crawl.js:527]
            ├─ scroll loop (maxPasses, waits, dedup, cursor)             [crawl.js:574-697]
            │    └─ emitProgress('scraping', items.length, maxItems)     [crawl.js:654]
            │         └▶ chrome.runtime → background.js
            │              └▶ POST /api/connectors/crawl-progress        [background.js:75-96]
            │                   └▶ agentConnectorCrawlProgress           [crawl.go:87-118]
            │                        └▶ ShouldEmitCrawlProgress (30s / +25) [notifications.go:32]
            │                             └▶ NotifyCrawlProgress (Telegram/feed) [notifications.go:382]
            └─ returns crawl_result{items, exit_reason, scroll_diag}
                 └▶ POST /api/connectors/crawl-result → processConnectorCrawlResult
                      └▶ leadingest.IngestPost … + NotifyCrawlSummary(exit_reason, scrollNote)
```

### Crawl-loop algorithm (`crawl.js:540-697`)

- **maxItems** `= clamp(task.crawl_plan.max_items, 1, 200)` → 50 here.
- **maxPasses** `= clamp(ceil(maxItems*3), 70, 260)` → **150**.
- **minPassesBeforeStop** `= clamp(ceil(maxItems*0.7), 18, maxPasses-1)` → **35**.
- **Scroll**: `findScrollTarget()` scores feed/main containers; `scrollByTarget()`
  dispatches a `WheelEvent` + `scrollBy({behavior:'smooth'})` + manual `scrollTop`.
- **Wait**: `waitMs = pass<8 ? 2200 : 3600`, plus a fixed `300ms` pre-grab sleep.
- **Extract**: `collectPostCandidates()` (full-doc `querySelectorAll` +
  `getBoundingClientRect` per candidate, every pass) → per-article
  content/author/permalink extraction.
- **Dedup**: in-memory `seen` Set keyed by `dedupKey()` (post permalink, else
  `c:djb2(author|content[:240])`). Ephemeral per crawl.
- **Progress count**: `items.length` (accepted unique posts), reported as `fetched`.
- **Stop**: `maxItems` reached · `cursor_match` (re-saw prior frontier post id) ·
  `no_progress` (`stagnantPasses>=10 && pass>=35`) · `no_new_items_after_scroll`
  (`pass-lastNewItemPass>=16 && pass>=35`) · `pass_exhausted` (hit 150).

---

## 2. Measured bottleneck hypotheses

Derived timing, not guessed. Per-pass wall time ≈ `300ms + waitMs` = **~2.5s**
(pass<8) / **~3.9s** (pass≥8). A full 150-pass run ≈ `8·2.5 + 142·3.9` ≈ **~9.6 min**.
Observed 22:47→23:08 = **~21 min** and only `10/50` — i.e. **longer than one full
pass budget**, so either the tab-timer waits stretched (background throttling) or the
task was re-dispatched. Both point at the same root: the loop makes almost no forward
progress per unit time.

| # | Suspected bottleneck | Evidence | Risk to change | Safe fix candidate | Validation needed |
|---|---|---|---|---|---|
| **H1** | **Background-tab throttling starves lazy-load.** Hidden/minimized FB tab → `requestAnimationFrame` + `behavior:'smooth'` scroll + `setTimeout` are throttled by Chrome; scroll barely moves, so FB never fetches the next page → posts trickle in over minutes. | Authors already suspected this: `scroll_diag.scroll_moved_ever` + comment `crawl.js:563-567` ("window minimized → rAF throttled, wrong scroll target"). 21 min ≫ 9.6 min ceiling ⇒ timer stretch. | Med — fixing properly means changing how scroll is driven. | **PR-C1**: surface `scroll_moved_ever` in the *in-flight* heartbeat so the operator sees "scroll not moving"; longer term drive scroll via CDP `Input.dispatchMouseWheel` on the worker path (no timer/visibility dependency). | Compare `scroll_moved_ever` true vs false against posts/min on a foreground vs background tab. |
| **H2** | **Fixed long waits regardless of load result.** Every pass sleeps 2.2–3.6s even when new posts already rendered; no adaptive shortening. | `crawl.js:695-696` — `waitMs` depends only on `pass`, never on whether the last scroll produced new posts. | Low — pure timing; must stay ≥ a safe floor (no speed-*increase* past current cadence on barren passes). | **PR-C1**: adaptive wait — keep 3.6s after a *barren* pass, drop to a floor (e.g. 1.2s) after a *productive* pass. Net effect on happy path only; barren pacing unchanged. | Unit test on a pure `nextWait(newItems, pass)` fn; soak on a live feed. |
| **H3** | **Lenient no-progress cutoff → ~1 min of dead scrolling before giving up.** | `no_progress` needs 10 stagnant passes (~39s) *and* `pass≥35`; `no_new_items_after_scroll` needs 16 barren passes (~62s). | Low-Med — earlier stop changes yield on slow-but-live feeds. | **PR-C1**: expose `no_progress_rounds` in status first (C0-spirit), then tune cutoff with tests. Improves *graceful stop*, not speed. | Characterization test pinning current exit reasons before tuning. |
| **H4** | **Live crawl loop does not consume the extension's existing checkpoint/login classifiers.** A soft-checkpoint / "going too fast" / login-wall interstitial yields 0 articles; the loop grinds all 150 passes then exits `pass_exhausted`/`no_progress` — never signalling risk. | The extension already classifies checkpoint/login elsewhere — `content/proof.js:87` (rate-limit/blocked/checkpoint banner text), `src/facebook-state.js:50` (`checkpoint`/`two_step` → `facebook_human_required`), `src/outbox.js:228` (`isLoginOrCheckpointUrl` → `human_required`). But `crawl.js` `crawlVisibleFacebookPosts` never reads any of them; its only guard is `locationMatchesExpected` (`wrong_page`), and the Go CDP `ErrFacebookCheckpoint` path is bypassed on this route. | Med — must be detect-and-stop only (no solving). | **PR-C2**: have the crawl loop consume the existing classifier signal (or a shared probe) → stop early, return typed `exit_reason:"checkpoint_suspected"`; server maps to `human_required` + notify (reusing `CheckpointManager` semantics). No new detection heuristic invented — reuse what proof/state/outbox already do. | Fixture HTML for checkpoint page → shared classifier returns true; behavior test that loop stops. |
| **H5** | **Per-pass full-document scan forces reflow.** `collectPostCandidates()` runs several document-wide `querySelectorAll` + `getBoundingClientRect` every pass. | `crawl.js:271-287` — unscoped `document.querySelectorAll` + rect reads each of 150 passes. | Low. | **PR-C1** (minor): scope candidate scan to the resolved scroll target / only newly-added nodes. | Micro-benchmark; result-parity test. |

**Primary suspect: H1** (throttled scroll starves lazy-load), amplified by **H2**
(fixed waits) and hidden by **H4** (a checkpoint would look identical to a slow feed).

---

## 3. Checkpoint / risk-handling assessment

- **Go CDP path**: correct. `FetchBatch` errors are matched via `runtime.CDPError`;
  `ErrFacebookCheckpoint` → `{"status":"human_required","reason":"facebook_checkpoint"}`,
  ban/logout → `aborted`, context drift → `aborted`. `session.CheckpointManager`
  transitions the session `active→checkpoint`, persists the URL, alerts via VNC, and
  **never retries or auto-solves** (`NO_AUTOMATED_CAPTCHA_BYPASS`). This is sound.
- **Extension path (the incident path)**: **classifiers exist but the crawl loop
  doesn't use them.** The extension already detects checkpoint/login/rate-limit signals
  in `content/proof.js:87`, `src/facebook-state.js:50` (`checkpoint`/`two_step` →
  `facebook_human_required`), and `src/outbox.js:228` (`isLoginOrCheckpointUrl` →
  `human_required`). But `crawlVisibleFacebookPosts` consumes **none** of them during a
  live crawl — its only guard is the `wrong_page` location check. So a checkpoint
  interstitial is indistinguishable from a slow feed: the loop scrolls for ~9.6 min and
  exits with a generic reason. The operator's "not convincing" complaint is accurate —
  the gap is specifically that the live crawl progress path never surfaces the
  already-classified signal as `checkpoint_suspected`.
- **Recommended (PR-C2)**, staying inside the safety boundary: have the crawl loop
  **consume the existing proof/state/outbox classifier** → **stop** → report
  `checkpoint_suspected` / `risk_blocked` → notify. No new heuristic, no auto-click, no
  solving, no rotation.

---

## 4. Data-plane assessment

- **Per-crawl dedup (`seen` Set)**: in-browser, ephemeral. Correct — local-runtime
  transient state, never persisted. No violation.
- **Recurring cursor (`org_crawl_intents.cursor_last_post_id/at`)**: defined in **both**
  the SQLite legacy baseline (`0001_legacy_baseline__sqlite`) and the platform Postgres
  migration (`0108_platform_crawl__postgres`); `store/crawl` is dialect-aware
  (`dbutil.Dialect`). This is the tenant recurring-intent frontier → **platform
  (Postgres) system-of-record** by doctrine; the SQLite copy is the mid-migration
  legacy plane (consistent with the ongoing database-boundary sprint, e.g. `a3c3b15`).
  **No PR-C0 action** — moving/merging planes is forbidden without stop-and-report, and
  nothing here blurs the planes at runtime.

---

## 5. Observability gaps (root of "5/50 for 16 minutes, no idea why")

In-flight heartbeat (`emitProgress` → `crawl-progress` → `NotifyCrawlProgress`) carries
only `{stage∈started|scraping|finished, fetched, max, source_url}`. During a stall,
`ShouldEmitCrawlProgress` still fires every 30s (`fetched` delta < 25) → the operator
gets ~32 identical `5/50` pings with **no cause**. Missing from the in-flight signal:

- `phase` (loading / scrolling / extracting / waiting / stalled / checkpoint_suspected)
- `new_count` vs `duplicate_count` (is it finding posts it already has?)
- `scroll_count`, `no_progress_rounds`, `scroll_moved_ever` (is our scroll even moving?)
- `last_new_post_at`, `last_error` / safe reason

The rich `scroll_diag` (`passes`, `max_articles_seen`, `scroll_moved_ever`,
`final_scroll_target`, `landed_url`) and `exit_reason` **exist but only in the final
`crawl-result`** (`NotifyCrawlSummary`) — too late to explain a live stall.

---

## 6. Ranked PR-C1 recommendations (impact ÷ risk)

1. **Enrich the in-flight heartbeat** with `phase` + `new_count`/`duplicate_count` +
   `no_progress_rounds` + `scroll_moved_ever`. All values already exist in the loop;
   this crosses the `crawl-progress` wire contract (extension `emitProgress` +
   background body + Go `agentConnectorCrawlProgress` struct + `NotifyCrawlProgress`
   text), so it needs a small, additive, tested wire change — **not** a C0 edit.
   *(Highest impact, low behavior risk; directly answers the operator complaint.)*
2. **Consume the existing checkpoint/login classifier + graceful stop** in the crawl
   loop (H4) — **PR-C2**. Reuse `proof.js`/`facebook-state.js`/`outbox.js` classification
   (no new heuristic); stop-only; reuse `CheckpointManager` server semantics. Tests:
   checkpoint fixture + loop-stops behavior test.
3. **Adaptive wait after productive passes** (H2). Pure `nextWait(newItems, pass)`
   helper + unit test; barren-pass pacing unchanged (no speed increase on risk paths).
4. **Tune no-progress cutoff** (H3) *after* #1 makes `no_progress_rounds` visible;
   guard with a characterization test pinning today's exit reasons first.
5. **Scoped candidate scan** (H5) — minor perf, result-parity test.

Each of #1–#5 is a separate small PR (one branch each). #2–#4 are behavior-changing →
must ship with tests protecting the new reason codes.

---

## 7. Validation run (PR-C0)

Audit-only: **zero production diff** — no runtime (JS/Go), schema, or wire-contract code
is changed. This PR touches only two governance-managed files: this doc and its
`specs/SPEC_REGISTRY.json` entry (registering a spec is part of docs governance, not a
runtime change). Confirmed green on branch:

- `python scripts/check_spec_registry.py` → PASS (53 entries, in sync; this doc registered)
- `bash scripts/check_docs_governance.sh` → OK
- `python scripts/check_file_size.py` → PASS (crawl.js unchanged at 727, allowlisted)
- `git status --short` → clean · `git diff --check` → clean
- `go build ./internal/jobhandlers/facebook_crawl/... ./internal/runtime/...
  ./internal/server/agent/crawlingest/... ./internal/session/...` → OK (no Go changed)

Sonar / CodeRabbit expectation: **no new code** in this PR beyond this doc → no new
issues; a docs-only change. The follow-up PR-C1 items carry their own tests + reason
codes and will be Sonar/CodeRabbit-reviewed individually.

## 8. Rollback

Docs-only branch. Rollback = delete the branch / revert this single file. No runtime,
schema, contract, or data-plane surface touched.
