# Account Safety — Technical Contract (PR-C0.5)

Track: **Facebook Automation Reliability**. Type: **architecture / code-spec baseline.**
Status: **draft — docs only, no runtime change.** Layer: **technical contract** for the
`account-safety` feature (domain: facebook-sales-intelligence; supports the
fresh-lead-discovery experience). Hard safety boundaries and data-plane ownership:
[decisions/safety-boundaries.md](./decisions/safety-boundaries.md). Companion to the
[Crawl Speed & Checkpoint Audit](../multi-group-fresh-lead-crawl/evidence/crawl-speed-checkpoint-audit.md) (PR-C0).

This spec defines the data model, state machine, algorithm boundaries, and ownership so
the PR-C1..C5 runtime work lands as small, testable, safe changes. It designs **account
safety, controlled concurrency, graceful stop, cooldown, and operator visibility** —
**not** anti-detection. Explicitly out of scope everywhere below: fingerprint spoofing,
stealth/evasion, proxy/account rotation to dodge checkpoint, CAPTCHA/checkpoint solving,
auto-clicking security challenges, and speed increases.

Grounding (already in the codebase — reuse, do not reinvent):
- Extension crawl path: `content/bridge.js` → `content/crawl.js`
  (`crawlVisibleFacebookPosts`) → `background.js` → `POST /api/connectors/crawl-progress`
  → `system.NotifyCrawlProgress`.
- Existing checkpoint/login classifiers: `content/proof.js:87`,
  `src/facebook-state.js:50`, `src/outbox.js:228`.
- Existing per-account lease: `session.Allocator.Acquire(accountID, PolicySticky, workerID)`
  / `Release` (`internal/session/allocator.go`).
- Existing browser-session state machine + checkpoint lifecycle:
  `session.StateMachine` (`ready/active/idle/recovering/checkpoint/terminated`),
  `session.CheckpointManager` (`internal/session/checkpoint.go`, `NO_AUTOMATED_CAPTCHA_BYPASS`).

---

## 1. Problem statement

A single operator machine may host **5–10 signed-in Facebook accounts** (one persistent
browser workspace each). Today a crawl is dispatched per account with no machine-level
coordination: nothing stops 5–10 accounts from actively scrolling group feeds at the same
time on the same host/IP.

Why blindly-parallel active crawls are risky:
- **Correlated risk.** Simultaneous automation on one machine/IP is a strong "coordinated
  inauthentic behaviour" signal; a checkpoint on one account raises the odds for its
  neighbours, so parallelism multiplies checkpoint exposure instead of adding throughput.
- **No back-pressure on a bad signal.** The audit showed a single crawl grinds ~9.6 min
  even when a feed is starved or a checkpoint interstitial is showing (`crawl.js` doesn't
  consume the existing classifiers). Run 10 of those at once and one checkpoint page can
  silently burn the whole fleet's time budget.
- **Retry storms.** Uncoordinated re-dispatch after a failure/checkpoint can hammer the
  same account or rotate straight to the next — exactly the pattern platforms flag.

THG's answer is **coordination via safety budgets, not evasion**: bound how much
automation runs per machine and per account, stop gracefully on risk, cool down, and make
the *why* visible to the operator. Cooldown is a safety brake, never a way to slip past a
platform check.

---

## 2. Target architecture: Account Safety Coordinator

A single host-scoped component (the **Account Safety Coordinator**, "Coordinator") that
decides *which* account may run automation *now*, and stops/paces work on safety signals.
It is a **decision + bookkeeping** layer that sits **above** the existing `session`
package; it does not touch the browser directly.

Responsibilities (owns):
- **Per-machine concurrency budget** — a hard cap on concurrently *running* crawls on this
  host (see §4).
- **Per-account lease** — at most one active automation workflow per account. Implemented
  by **reusing** `session.Allocator.Acquire(accountID, PolicySticky, workerID)` /
  `Release`; the Coordinator is the single caller that gates acquisition on budget.
- **Per-account risk budget** — the counters in §5; when a budget trips, reduce activity or
  enter cooldown.
- **Cooldown** — a per-account "do not schedule until `cooldown_until`" timer with a
  reason.
- **Checkpoint/login/risk stop policy** — on a classified signal, stop the account's work
  and transition it to the right terminal-ish state (§3); never solve, never bypass.
- **Pause-reason telemetry** — every non-`running` state carries a machine-readable
  `reason_code` surfaced to the operator (§6).
- **Safe scheduler decision** — a pure function `nextAccountToRun(state) -> accountID | none`
  over the queue + budgets; no hidden sleeps, no side effects.

Hard boundaries (what the Coordinator must NOT own — no evasion, no solving, no
rotation, no session-lifecycle ownership, no cross-plane writes) are the decision
record [decisions/safety-boundaries.md](./decisions/safety-boundaries.md), which
also owns the data-plane ownership doctrine formerly in §7 of this file.

---

## 3. State machine

Per-**account automation-runtime** state (distinct from, and layered above, the existing
`session.StateMachine` browser states). The Coordinator owns these; `checkpoint_required`
/`login_required`/`human_required` map onto the existing `session` `checkpoint` semantics
so `CheckpointManager` remains the single checkpoint authority.

```text
 ready ──enqueue──▶ queued ──lease+budget──▶ running
   ▲                  │                         │
   │                  │                 ┌───────┼─────────────────────────────┐
   │                  │                 ▼       ▼                             ▼
   │           (budget/cooldown)  cooling_down  stalled_no_progress   checkpoint_required
   │                  ▲                 │       │                     login_required
   │                  └─────────────────┘       │                     risk_blocked
   └──────────────(cooldown elapsed)────────────┘                             │
                                                                        human_required
```

| State | Entry condition | Allowed next states | Forbidden actions | Operator message |
|---|---|---|---|---|
| **ready** | Account eligible; no cooldown; not leased. | `queued` | Starting work without going through `queued` (skips budget check). | "Sẵn sàng." |
| **queued** | Enqueued for a crawl/workflow; waiting for a lease + machine budget slot. | `running`, `cooling_down` (if a budget trips while waiting), `ready` (dequeued) | Running before a lease + budget slot is granted. | "Đang chờ tới lượt (N tài khoản trước)." |
| **running** | Holds the per-account lease **and** a per-machine concurrency slot. | `ready` (clean finish), `cooling_down`, `stalled_no_progress`, `checkpoint_required`, `login_required`, `risk_blocked` | Starting a 2nd workflow on this account; ignoring risk-budget trips. | "Đang chạy: <phase>, <fetched>/<max>." |
| **cooling_down** | A risk budget tripped, or a clean run finished with pacing → wait until `cooldown_until`. | `ready` (timer elapsed), `queued` | Scheduling this account before `cooldown_until`; treating cooldown as a retry knob to push past a check. | "Tạm nghỉ an toàn tới <time> (lý do: <reason>)." |
| **stalled_no_progress** | `no_progress_rounds` exceeded the safe cutoff with no new items. | `cooling_down`, `checkpoint_required` (if a classifier fires), `ready` | Infinite scrolling; increasing speed to "unstick". | "Không có tiến triển — dừng an toàn." |
| **checkpoint_required** | A checkpoint classifier (`proof.js`/`facebook-state.js`/`outbox.js`) fired. Maps to `session` `checkpoint`. | `human_required` | Auto-solving, auto-clicking, continuing to crawl, switching to another account to "route around" it. | "Cần xác minh thủ công (checkpoint)." |
| **login_required** | A login-wall classifier fired (`isLoginOrCheckpointUrl`, `facebook_login_required`). | `human_required` | Auto re-login with stored secrets; continuing to crawl. | "Cần đăng nhập lại thủ công." |
| **risk_blocked** | Rate-limit / "going too fast" / block banner classified, or risk budget hard-tripped. | `cooling_down`, `human_required` | Bypassing, speeding up, rotating accounts to dodge the block. | "Tạm khóa vì rủi ro — nghỉ an toàn." |
| **human_required** | Terminal until an operator acts (checkpoint/login/hard risk). Reuses `CheckpointManager` alert + VNC deep-link. | `ready` (operator resolved, verified) | Any automated resolution; auto-transition back to `ready` without operator/verifier confirmation. | "Đang chờ người xử lý. Mở VNC để xác minh." |

Invariant: the **only** path back into `ready` from a risk/checkpoint state is
operator-driven and verifier-confirmed (existing `CheckpointManager.ResolveCheckpoint`
semantics). No timer or scheduler ever auto-clears `human_required`.

---

## 4. Concurrency policy

Safe defaults (conservative; tune only later with telemetry evidence, §8):
- **`max_active_crawls_per_machine` = 1** (default). 2 permitted only as an explicit,
  operator-set opt-in; never higher without evidence.
- **`max_active_workflows_per_account` = 1** — enforced by the per-account lease
  (`PolicySticky`).
- **Queue the rest.** Accounts beyond the machine budget sit in `queued`, FIFO, and are
  admitted one at a time as slots free.
- **No immediate account switching after a checkpoint/risk wall.** When an account hits
  `checkpoint_required`/`login_required`/`risk_blocked`, the Coordinator does **not**
  instantly promote the next queued account into the freed slot; it applies a short
  machine-level settle delay first. Switching instantly is a rotation-to-dodge pattern and
  is forbidden.
- **No retry storm.** A failed/stopped account re-enters via `cooling_down` with backoff,
  not immediate re-dispatch. Backoff is bounded and reason-tagged.
- **No parallel crawl + outbound on the same account** in this phase. One workflow class at
  a time per account; combined pipelines are a later, explicit decision (not assumed here).

All of the above are **admission decisions** computed before any browser work starts —
pure policy over counters, no sleeps embedded in business logic.

---

## 5. Risk budget policy

Per-account counters (names are the contract for PR-C1 telemetry and PR-C3 policy):

| Counter | Meaning | Plane |
|---|---|---|
| `crawl_duration_ms` | Wall time of the current/last crawl. | Local (ephemeral) |
| `scroll_count` | Scroll passes performed this run. | Local |
| `no_progress_rounds` | Consecutive passes with no new item. | Local |
| `duplicate_count` | Items seen but already known (dedup hits). | Local |
| `failed_extraction_count` | Articles that yielded no usable post. | Local |
| `checkpoint_login_suspicion_count` | Times a checkpoint/login classifier fired. | Local → summarized durably |
| `recent_automation_window` | Rolling count of automation runs in the last window. | Durable (platform) |
| `cooldown_until` | Timestamp before which the account must not be scheduled. | Durable (platform) |
| `last_safe_stop_reason` | Reason code of the last non-clean stop. | Durable (platform) |

Policy:
- Budgets **reduce activity or enter cooldown** — they lower cadence or move the account to
  `cooling_down`/`risk_blocked`. They never raise speed or concurrency.
- Budgets **never attempt to bypass** a platform check. A tripped budget is a brake, not a
  workaround.
- **Cooldown is safety, not evasion.** Its purpose is to lower correlated risk on the
  machine, not to wait out a rate limiter to then resume harder.
- Budget evaluation is a **pure function** `evaluateRisk(counters) -> decision{action,
  reason_code, cooldown_until?}` — trivially unit-testable, no I/O.

Thresholds are declared as named constants with conservative defaults and are only changed
in PR-C4 under telemetry evidence, never speculatively.

---

## 6. Progress telemetry alignment (contract for PR-C1)

The live progress signal (extends today's `{stage, fetched, max, source_url}` heartbeat)
should expose:
- `active_account` (id) and `queued_account_count`
- `phase` — loading / scrolling / extracting / waiting / stalled / checkpoint_suspected
- `fetched_count`, `new_count`, `duplicate_count`
- `no_progress_rounds`
- `scroll_moved_ever` (already computed in `crawl.js` `scroll_diag`)
- `risk_state` (the §3 state)
- `cooldown_reason` / `cooldown_until`
- `safe_reason_code` (typed; see privacy rules)

Privacy rules (binding on every field above):
- **No raw checkpoint/challenge text.** Only a typed `reason_code`
  (e.g. `checkpoint_suspected`) leaves the browser — never the interstitial's copy.
- **No cookies / session secrets / credential material**, ever, in any telemetry.
- **No full-page DOM** or scraped page HTML in progress/telemetry payloads.
- **No cross-account/cross-org leakage** — a heartbeat carries only the owning account's
  state, routed to the owning member (consistent with the existing
  `recordAutomationForAccount` account-privacy routing).

Reason codes are the interface; raw text is an implementation detail that must not escape.

---

## 7. Data-plane ownership

Moved to the decision record:
[decisions/safety-boundaries.md](./decisions/safety-boundaries.md) owns the
data-plane ownership doctrine (local ephemeral vs SQLite local runtime vs
PostgreSQL platform vs RAG; no browser secrets server-side; no plane move
without a dedicated migration PR; cross-plane flow via the explicit
event/outbox path only).

---

## 8. Clean Code / algorithmic rules

Clean Code here means the *right model*, so future functions are naturally small:
- **State machine over scattered booleans.** One `AccountRuntimeState` enum + a transition
  table, not `isRunning`/`isCoolingDown`/`isBlocked` flags that can contradict each other.
- **Queue + lease over ad-hoc goroutines.** Admission is a FIFO queue guarded by the
  existing `Allocator` lease; no free-spawning goroutines per account, no shared mutable
  concurrency counters outside the Coordinator.
- **Reason codes over raw text logs.** Decisions carry typed codes; raw page/DOM text is
  never a control signal or a telemetry field.
- **Map/set for dedup & membership.** Seen-post dedup and "is this account queued/leased"
  are set/map lookups, not list scans.
- **Pure policy helpers.** `nextAccountToRun`, `evaluateRisk`, `nextWait`, and the
  state-transition function are pure (state in → decision out), so tests need no browser,
  no DB, no clock injection beyond a passed-in `now`.
- **DB constraints/leases where durable coordination matters.** Per-account single-active
  is enforced by the lease, not by hopeful application checks.
- **No generic scheduler framework until needed.** A FIFO queue + budget checks is the
  whole scheduler for now; no plugin/priority/cron abstraction until a real second use case
  exists.
- **No performance cleverness without telemetry evidence.** The §9 perf PR (C5) is gated on
  telemetry proving the DOM scan is the bottleneck; until then, correctness and safety win.

---

## 9. Future PR train

| PR | Scope | Behavior change? |
|---|---|---|
| **PR-C1** | Live crawl telemetry only — surface `phase`/`new`/`duplicate`/`no_progress_rounds`/`scroll_moved_ever` over the `crawl-progress` wire. | Additive telemetry; no crawl behavior change. |
| **PR-C2** | Crawl loop **consumes the existing** checkpoint/login classifier (`proof.js`/`facebook-state.js`/`outbox.js`) + graceful stop → typed `checkpoint_suspected`; reuse `CheckpointManager`. | Adds a stop; no bypass, no new heuristic. |
| **PR-C3** | Account Safety Coordinator — minimal scheduler + per-account lease (reuse `Allocator`) + per-machine budget + cooldown state. | New coordination; no speed/concurrency increase (defaults tighten, not loosen). |
| **PR-C4** | Safe pacing / no-progress cutoff tuning **driven by PR-C1 telemetry**. | Pacing change with tests; never a raw speed increase on risk paths. |
| **PR-C5** | Extraction perf cleanup **iff** telemetry proves the DOM-scan is the bottleneck. | Perf only, result-parity tested. |

Each is one branch/PR; C2–C4 are behavior-changing and ship with tests protecting their
reason codes and state/budget decisions.

---

## 10. Review checklist

Moved to the runbook layer:
[runbooks/review-checklist.md](./runbooks/review-checklist.md) — the 9-item
safety checklist applied to every PR-C* runtime PR (speed, concurrency,
checkpoint behavior, bypass/evasion, lease, data-plane, text leakage, pure
policy testability, Sonar/CodeRabbit hygiene).

---

## Rollback

Docs-only spec. Rollback = revert this file + its `SPEC_REGISTRY.json` entry. No runtime,
schema, contract, or data-plane surface is touched by this PR.
