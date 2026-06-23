# Append-Only Ledger Migration — Design

**Status**: design (no code). Stage 3 §5 semantic cleanup deliverable per [RUNTIME_TOPOLOGY.md §4](../platform/RUNTIME_TOPOLOGY.md#4-append-only-boundaries).

**Why this doc exists**: `action_ledger` is supposed to be append-only per [[feedback_append_only_correction_events]]. After the Phase 5B coordination extraction shipped, three UPDATE call sites remain (counted by `scripts/check_topology.sh` §6.6 as `EXPECTED-FAIL` with baseline 3). This doc proposes the migration path. Implementation is a separate PR — this is the design-first artifact the user mandated for high-risk semantic changes.

---

## 1. Current state — the three violations

| # | Location | UPDATE shape | Caller |
|---|----------|--------------|--------|
| 1 | `coordination/action_ledger.go:188` `MarkActionLedgerOutcome(ctx, ledgerID, outcome, reason)` | `UPDATE action_ledger SET outcome=?, reason=? WHERE id=?` | Manual admin ops (rare) |
| 2 | `coordination/action_ledger.go:229` `MarkActionLedgerOutcomeByOutbound(ctx, orgID, outboundID, outcome, reason)` | `UPDATE action_ledger SET outcome=?, reason=? WHERE id=?` (after SELECT MAX(id) for that outbound) | `outbox_agent.go::finalize` — every verified outcome | Hot path |
| 3 | `coordination/engagement_reconcile.go:165` `ReconcileEngagement(ctx, orgID)` correction loop | `UPDATE action_ledger SET outcome='failed', reason='reconciled:...' WHERE id=? AND outcome='succeeded'` | Scheduled reconcile job |

**What each is trying to express**:

1. + 2 are the **lifecycle flip** — `queued` → (`succeeded` | `failed`) when the executor finishes. Currently overwrites the original 'queued' row.
2. is the **HOT** one: every outbound finalize path triggers it. Volume scales with execution rate.
3. is the **correction** — engagement projection said someone got commented, reconcile finds the proof says otherwise, ledger needs to reflect the real verdict.

---

## 2. Target shape — event-sourced ledger

Replace in-place UPDATE with **append a new event row whose `target_outbound_id` links back to the original**. Projection logic computes "current outcome" as `MAX(id)` per outbound across event types.

### 2.1 Schema additions (additive — no break)

```sql
-- New column on action_ledger:
ALTER TABLE action_ledger ADD COLUMN event_type TEXT NOT NULL DEFAULT 'action_attempted';
-- Existing rows backfill to 'action_attempted' which is semantically
-- what they were (an attempted action with intent='queued').

-- New event_type values:
--   action_attempted      — original write (was the only row type)
--   outcome_classified    — verifier classification arrived
--   engagement_revoked    — reconcile correction event

CREATE INDEX idx_action_ledger_event_outbound
  ON action_ledger(outbound_id, event_type, performed_at DESC)
  WHERE outbound_id > 0;
```

No DROP. No ALTER on existing column types. Existing readers continue to work because they query `outcome` regardless of `event_type` — they just see only `action_attempted` rows during the additive phase.

### 2.2 New writer APIs (coordination)

```go
// Append-only. Replaces MarkActionLedgerOutcomeByOutbound for the
// verifier finalize path.
//
// Inserts an `outcome_classified` event. Does NOT touch the original
// `action_attempted` row.
func (s *Store) RecordOutcomeForOutbound(
    ctx context.Context,
    orgID, outboundID int64,
    outcome, reason string,
) (eventID int64, err error)

// Reconcile-side correction. Inserts an `engagement_revoked` event
// when the reconciliation pass finds a mismatch.
func (s *Store) RecordEngagementRevoked(
    ctx context.Context,
    orgID, outboundID int64,
    actualOutcome, originalLedgerID int64, // back-ref to the row being corrected
    reason string,
) (eventID int64, err error)
```

`MarkActionLedgerOutcome` and `MarkActionLedgerOutcomeByOutbound` get marked `// Deprecated:` and forward to the new APIs internally (transition phase), then are removed in a follow-up.

### 2.3 New reader contract — projection over events

`ListActionLedger` continues to return rows but its semantics shift: each row is an EVENT, not "the current state of an action". For "what is the current outcome of outbound X" callers move to:

```go
// CurrentOutcomeForOutbound returns the most-recent event-derived
// outcome for an outbound. Picks the latest event by id (monotonic).
// Returns ("", nil) when no events exist for that outbound.
func (s *Store) CurrentOutcomeForOutbound(ctx context.Context, orgID, outboundID int64) (string, error)
```

Engagement projection (`leads.GetLeadEngagement` etc.) switches to query through `CurrentOutcomeForOutbound` for each outbound it's projecting, instead of trusting `action_ledger.outcome` directly.

---

## 3. Staged rollout

Per [[feedback_staged_evolution_over_big_bang]] this lands additive → cleanup. Two PRs:

### PR A — Additive (zero behaviour change for current readers)

1. Schema ALTER: add `event_type` column with default `action_attempted`. Backfill existing rows (default handles new rows).
2. New writer APIs added on `coordination.Store`. New `event_type` constants exported.
3. Existing `MarkActionLedgerOutcome*` methods stay AS-IS — still do their UPDATE. (Append-only invariant still violated; `check_topology.sh` baseline still 3.)
4. New `CurrentOutcomeForOutbound` reader added (computes from events).
5. Tests pin the dual representation: writing via new API + reading via new reader gives same result as writing via old UPDATE + reading the column.

**Verification gates**: existing tests stay green. New tests cover the new APIs. Topology gate baseline unchanged.

### PR B — Reader migration (zero writer change)

1. Engagement projection switches to `CurrentOutcomeForOutbound`.
2. `GetLeadEngagement` and any callers reading `action_ledger.outcome` directly migrate.
3. `MarkActionLedgerOutcome*` still update the column for back-compat.
4. Now readers and writers agree even if multiple events exist per outbound.

**Verification gates**: same as PR A + the lead-engagement-projection integration tests still green with both writer paths active.

### PR C — Writer cutover (the UPDATE removal)

1. `finalize` path in `outbox_agent.go` switches from `MarkActionLedgerOutcomeByOutbound` to `RecordOutcomeForOutbound`.
2. `ReconcileEngagement` switches from UPDATE to `RecordEngagementRevoked`.
3. `MarkActionLedgerOutcome*` removed (or marked deprecated with eventual deletion).
4. `check_topology.sh` baseline drops from 3 to 0; gate moves from `EXPECTED-FAIL` to `PASS`.

**Verification gates**: full test suite + topology gate `PASS`. A staging soak to confirm projection-derived outcomes match the prior column-derived outcomes.

---

## 4. Risks + mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| Reader sees old `outcome` column during PR B before switching to projection | High (silent inconsistency) | Dual-write window short — PR B must close within one cycle. Add a CI test that fails if both writer paths produce divergent outcomes. |
| Performance: `MAX(id)` per outbound for every projection read | Medium | Index `(outbound_id, event_type, performed_at DESC)` covers the lookup; benchmark before+after; if hot path slow, add a materialised `outbound_messages.last_event_id` column. |
| Reconciliation idempotency: re-running reconcile shouldn't append duplicate `engagement_revoked` events | Medium | Reconcile pre-check: `SELECT 1 FROM action_ledger WHERE outbound_id=? AND event_type='engagement_revoked'` — skip if exists. |
| Historical rows have only `action_attempted` events with mutated `outcome` field | Low | The backfill leaves them as-is. The new `CurrentOutcomeForOutbound` reader gracefully returns the row's existing outcome when no later event exists. |
| L2 wrapper count grows during transition (Mark* still deprecated-present) | Low | Track via `check_topology.sh` §6.9 informational counter. PR C removes them. |

---

## 5. Out of scope

- Migrating `execution_attempts` UPDATEs (the `Begin`→`AdvanceStatus`→`Finish` lifecycle UPDATEs). Those are intra-row state and the design intent — see [RUNTIME_TOPOLOGY.md §4](../platform/RUNTIME_TOPOLOGY.md#4-append-only-boundaries) "Append-only by design" table.
- Migrating `account_runtime_state` writes. That's a counter table, not a ledger; UPDATE is the right semantics.
- Reworking `outbound_messages.execution_state` CAS. That's the state machine primitive and stays as-is per the V2 outbound refactor design.

---

## 6. Acceptance criteria for "migration done"

- `scripts/check_topology.sh` §6.6 reports `PASS` (baseline 0).
- Zero `UPDATE action_ledger` or `DELETE FROM action_ledger` statements in production code (only `INSERT`).
- `MarkActionLedgerOutcome` + `MarkActionLedgerOutcomeByOutbound` removed.
- Engagement projection re-derives state from event sequence and matches the previous behaviour on historical data (replay test on a captured production DB snapshot).

This unblocks Stage 3 §5 fully and frees the system to support correction events from sources beyond reconciliation (e.g. operator-initiated overrides, manual badge resets).
