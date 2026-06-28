---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-28
related_pr_or_issue: chore/docs2-architecture-backlinks-frontmatter
---

# Connector State Machine & Action Lifecycle

> Part of the [architecture docs index](INDEX.md).

**Status:** OFFICIAL STANDARD. **Companion of** `ARCHITECTURE_STANDARD.md`.
Defines connector states, the outbound action lifecycle, and the pull-based
execution contract. Grounds the existing readiness logic
(`internal/store/connectors.PickReadyConnector`, `browsergateway.StreamFacebookLoggedIn`)
in one normative model.

---

## 1. Connector states

A connector is one paired Chrome extension bound to one Facebook account. Its state
is derived from heartbeat + chrome-status + stream-status. States:

| State | Meaning | Eligible to claim work? |
|---|---|---|
| `offline` | no recent heartbeat | no |
| `heartbeat_stale` | heartbeat older than the freshness window (e.g. >5 min) | no |
| `ready` | online, FB tab logged in, idle | **yes** |
| `busy` | online but executing a claimed job | no (already working) |
| `login_required` | online but FB session not logged in | no → surface "log in" |
| `challenge_required` | FB checkpoint/captcha/2FA wall | no → `human_required` |
| `blocked` | account blocked/disabled by the platform | no |
| `rate_limited` | backing off after platform rate signals | no until cooldown |
| `unknown` | state cannot be determined yet | no (fail safe) |

**Transitions (normative):**

```
offline ──heartbeat──▶ heartbeat_stale ──fresh──▶ (login check)
                                                    │
        ┌───────────────────────────────────────────┤
        ▼                  ▼               ▼          ▼
   login_required     challenge_required  ready ◀──done── busy
        │ (user logs in)   │ (human solves)  │ claim job ─▶ busy
        └──────────────────┴────────────────▶│
                                             ▼
                                  rate_limited / blocked (platform signal)
```

- `ready` is the ONLY state from which the server hands out work.
- `challenge_required` and `login_required` are **human-required** terminal-until-
  resolved states: the system returns `human_required`, never auto-bypasses a wall.
- Any state may drop to `offline`/`heartbeat_stale` when heartbeats stop.

**Fail-safe rule:** when state is `unknown`, treat as not-ready. Never claim on a
guess. (This is the `#49-connector / #50-mission` lesson: the picker MUST agree with
the dispatcher on which account a connector actually serves.)

## 2. Action lifecycle (outbound)

An outbound action (comment / inbox / post) moves through:

| Stage | Owner write | Notes |
|---|---|---|
| `planned` | outbound (queue) | row created with `execution_state='planned'`; default approval-required where policy says so |
| `claimed` | outbound (CAS/lease) | exactly one connector claims via row-level CAS on `planned`; lease expiry set |
| `executing` | outbound | connector is performing the action in the browser |
| `reported` | outbound (from connector result) | connector posted result; raw outcome recorded |
| `verified` | coordination (append-only ledger) | independent verification confirms the action truly happened |
| `failed` | outbound + coordination | execution or verification failed; reason coded |
| `retry_scheduled` | outbound | eligible for another attempt after backoff |
| `cancelled` | outbound | superseded/expired/operator-cancelled |

- Truth split (existing): `execution_state` ⟂ `verification_outcome`. "queued ≠
  posted"; success counts only when **verified**.
- `execution_attempts` + `action_ledger` are **append-only**, written by
  `coordination` only. A correction is a new `engagement_revoked` row, not an UPDATE.
- Each terminal transition SHOULD emit a durable event (`CommentActionPosted`,
  etc.) via the outbox (`TRANSACTIONAL_OUTBOX.md`).

## 3. Pull-based execution contract

```
1. Server PLANS work          → outbound_messages(execution_state='planned')
2. Connector PULLS             → GET /connectors/outbox  (returns claimable work)
3. Server CLAIMS with CAS      → UPDATE ... SET execution_state='executing',
                                  claimed_by=connector, lease_expiry=now+T
                                  WHERE execution_state='planned'  (row-level CAS)
4. Connector EXECUTES in browser
5. Connector REPORTS           → POST /connectors/outbox/:id/sent | /failed
6. Server VERIFIES             → async reverify → append ledger → verified|failed
7. Lease expiry safety net     → stale 'executing' rows reset to 'planned' for retry
```

**Invariants:**

- **Connectors pull; the server never pushes execution.** A connector asks for work;
  it cannot be commanded to act outside its own pull.
- **CAS/lease prevents double-claim.** Two browser tabs or two devices bound to the
  same account cannot both claim the same job — the row-level CAS on `planned` lets
  exactly one win; the other sees the row already `executing`.
- **Heartbeat/readiness gates eligibility.** Only a `ready` connector (§1) is offered
  work. Readiness is computed by the shared `PickReadyConnector` so the create-time
  preflight and the run-time picker never diverge.
- **Lease expiry = liveness.** If a connector claims then dies, the lease expires and
  the row returns to `planned` for another `ready` connector. Idempotency at the
  verification layer prevents a double-post if the dead connector actually succeeded.
- **No secrets in logs.** Cookies, tokens, and session values are NEVER written to
  logs, command payloads visible to the dashboard, event payloads, or SQL results.
  Evidence-on-failure (screenshots) is preserved, but credentials are not.

## 4. `human_required` contract

When a connector hits `login_required` or `challenge_required`, the action does NOT
fail silently and is NOT bypassed:

- the action stays `planned` (or `retry_scheduled`) — not consumed;
- a `ConnectorChallengeRequired` event is emitted → notifications surface
  `human_required` to the operator;
- automation resumes only after a human resolves the wall and the connector returns to
  `ready`.

## 5. Mapping to current code

| Concept | Current code |
|---|---|
| readiness decision | `internal/store/connectors.PickReadyConnector`, `ConnReady` |
| logged-in stream state | `browsergateway.StreamFacebookLoggedIn` |
| pull endpoints | `internal/server/agent` `/connectors/outbox`, `/commands` |
| claim/lease/CAS | `internal/store/outbound` `claim.go`, `lease.go`, `transition.go` |
| append-only verify | `internal/store/coordination` `execution_attempts.go`, `action_ledger.go`, reverify |
| evidence on failure | connector screenshots (`connector_screenshots`) |
