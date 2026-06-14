# Transactional Outbox & Process Managers

**Status:** OFFICIAL STANDARD (target). **Companion of** `ARCHITECTURE_STANDARD.md`.
Defines how critical cross-module side effects are made **durable and exactly-once-
ish**, replacing the current composition-root-callback fan-out.

> **Today's reality (the gap this closes):** there is NO durable outbox.
> `internal/events/bus.go` is an in-memory SSE bus that **drops events on a slow
> consumer**. Cross-module reactions (e.g. "post lead created → resume comment") are
> wired as direct Go callbacks injected at the composition root (e.g. the P1
> `SetPostLeadImportHook`/`OnLeadCreated` chain). If the process restarts between the
> state change and the callback, the side effect is lost. `runtime_events` exists but
> is an audit stream, not a relayed outbox. This document is the durable replacement.

---

## 1. Why direct cross-module DB writes / callbacks are dangerous

Consider "import created a post lead → queue a comment":

- **Lost on crash.** State committed (lead row) but the process dies before the
  in-memory callback runs → the comment never queues. No record it was owed.
- **Dual-write inconsistency.** Writing the lead in one tx and the comment in another
  (or via an HTTP hop) can leave one without the other.
- **No retry / no idempotency.** A callback that fails has nowhere to be retried from,
  and a re-delivered event can double-fire (double comment).
- **Hidden coupling.** The producer module ends up holding a callback into the
  consumer, inverting the dependency and coupling failure domains (the crawl handler
  knowing about comment orchestration).

The outbox removes all four: the event is written **in the same transaction** as the
state change, so it cannot be lost; a relay delivers it at-least-once; consumers are
**idempotent** by event id; retries are bounded and observable.

## 2. Required outbox table shape

```
CREATE TABLE outbox_events (
    id              INTEGER PRIMARY KEY,        -- monotonic, relay cursor
    event_id        TEXT NOT NULL UNIQUE,       -- ULID/UUID; the idempotency key
    org_id          INTEGER NOT NULL,           -- tenant scope (every row)
    type            TEXT NOT NULL,              -- e.g. "FacebookLeadCreated"
    aggregate       TEXT NOT NULL,              -- e.g. "lead", "outbound_message"
    aggregate_id    TEXT NOT NULL,              -- the owning row id
    payload         TEXT NOT NULL,              -- JSON envelope (NO secrets)
    status          TEXT NOT NULL DEFAULT 'pending', -- pending|published|failed|dead
    attempts        INTEGER NOT NULL DEFAULT 0,
    available_at    TIMESTAMP NOT NULL,         -- backoff: next eligible publish time
    created_at      TIMESTAMP NOT NULL,
    published_at    TIMESTAMP,
    last_error      TEXT
);
CREATE INDEX idx_outbox_pending ON outbox_events(status, available_at);
CREATE INDEX idx_outbox_org ON outbox_events(org_id, type);
```

**Write rule:** the producer writes its state change AND the `outbox_events` row in
**one DB transaction**. Owner of the table: the `events` module. Producers insert via
`events.Emit(tx, evt)` — passing the open transaction, never a separate connection.

## 3. Event envelope

```json
{
  "event_id": "01J...ULID",
  "type": "FacebookLeadCreated",
  "org_id": 42,
  "occurred_at": "2026-06-14T09:00:00Z",
  "aggregate": "lead",
  "aggregate_id": "12345",
  "data": {
    "lead_id": 12345,
    "source_type": "post",
    "post_fbid": "456",
    "canonical_url": "https://www.facebook.com/groups/123/permalink/456/"
  },
  "trace_id": "..."
}
```

- **`data` carries ids and references, not secrets.** No cookies, tokens, session
  values, or full message bodies that could contain PII beyond what's needed.
- `org_id` is mandatory and is the only tenant a consumer may act within.
- `type` is a stable contract string (versioned by suffix if it must change, e.g.
  `FacebookLeadCreated.v2`).

## 4. Idempotency key

- The consumer records processed `event_id`s (a small `consumed_events(consumer,
  event_id)` table or a status column on the consumer's own aggregate).
- A re-delivered event whose `event_id` is already consumed is a **no-op**.
- This is what makes at-least-once delivery safe: the relay may deliver twice; the
  consumer acts once. (It replaces the P1 prototype's "consume-once delete" of a KV
  key, which is not crash-safe across the deliver/act boundary.)

## 5. Event relay

A single background loop (in the API process, or a dedicated `cmd/relay`):

```
loop:
  rows = SELECT * FROM outbox_events
         WHERE status='pending' AND available_at <= now
         ORDER BY id LIMIT N        -- claim with UPDATE ... RETURNING / CAS
  for evt in rows:
     deliver(evt)  -> in-process subscribers (process managers) and/or SSE
     on success: status='published', published_at=now
     on failure: attempts++, status='pending',
                 available_at = now + backoff(attempts),
                 last_error=...; at attempts>=MAX -> status='dead'
```

- **In-memory Go channels may be used only AFTER a durable row exists.** The relay
  reads the durable table and *then* fans out to in-process subscribers. Channels are
  a delivery optimization, never the source of truth. A dropped channel send is
  recovered on the next relay pass because the row stays `pending` until acked.
- The SSE bus (`internal/events/bus.go`) becomes a *subscriber* of the relay, not the
  event store.

## 6. Process manager (saga)

A process manager is a **consumer** that reacts to events and issues commands. It
lives in the owning *service* module (e.g. `services/facebook`), NOT in `events` and
NOT in the producer.

Example — direct-comment import continuation, done right (replacing the P1 prototype):

```
on FacebookPostImported(lead_id, post_fbid, canonical):
    if no pending CommentAfterImport for (org, post_fbid): return   -- idempotent
    cmd = PlanComment{org, lead_id, user_id, account_id}
    queueLeadOutreach(cmd)            -- same readiness/coverage/dedup gates
    mark continuation satisfied        -- in the process-manager's own table
```

The continuation state lives in a **process-manager table** owned by
`services/facebook`, not in `user_context`. The trigger is the durable
`FacebookPostImported` event, not an `OnLeadCreated` callback — so it survives a
restart and works regardless of which ingestion path (connector or worker) created
the lead.

## 7. Retry / failure semantics

| Concern | Rule |
|---|---|
| Delivery | at-least-once; consumers idempotent by `event_id` |
| Backoff | exponential on `available_at` (e.g. 1s, 4s, 16s, …) capped |
| Poison events | after `MAX_ATTEMPTS` → `status='dead'` + alert; never silently dropped |
| Ordering | per-aggregate best-effort via `ORDER BY id`; consumers must not assume global order |
| Tenant safety | a consumer acts only within `evt.org_id` |
| Observability | `dead`/high-`attempts` rows are a monitored signal |

## 8. Critical events (the contract set)

These MUST flow through the outbox (durable), never via ad-hoc callback:

| Event | Producer | Typical consumer(s) |
|---|---|---|
| `FacebookLeadCreated` | leadingest (in lead-insert tx) | notifications (channel), services/facebook (coverage) |
| `FacebookLeadScored` | ai/scoring via services | leads projection, work queue |
| `FacebookPostImported` | services/facebook import workflow | comment-continuation process manager |
| `CommentActionPlanned` | outbound (in queue tx) | connector dispatch, notifications |
| `CommentActionPosted` | outbound (on verified) | attribution, notifications, knowledge (outcome learning) |
| `ConnectorChallengeRequired` | connectors (heartbeat) | notifications (`human_required`), readiness |
| `ConnectorReadyChanged` | connectors (heartbeat) | services readiness, scheduler eligibility |

## 9. Migration note (do not big-bang)

This is roadmap Phase E. Introduce the `outbox_events` table + relay **additively**
alongside the existing in-memory bus; migrate one event at a time (start with
`FacebookPostImported`, which the P1 prototype needs). The in-memory bus keeps
working for non-critical SSE until each critical event is moved. No behavior changes
land in the same PR as the table/relay introduction.
