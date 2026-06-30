---
id: ARCHST-R1
status: DONE
lane: RED
risk: RED
depends_on: []
parallel_safe: false
branch: "audit/archst-r1-append-only-ledger-decision"
pr_url: https://github.com/CONG-TY-TNHH-QU-C-ANH-GLOBAL/THG_tool/pull/192
---

# ARCHST-R1 — AUDIT: append-only ledger UPDATE violations

## Goal (audit-only — DO NOT implement)
Decision record for the documented append-only violations: MarkActionLedgerOutcome* and engagement_reconcile issue UPDATEs against action_ledger, which downstream projections (leads.engagement_state) read.

## Component / domain
store/coordination truth ownership (action_ledger). RED.

## Files likely involved
coordination/action_ledger.go, coordination/engagement_reconcile.go, a new corrections table + migration.

## Dependencies
Drives specs/APPEND_ONLY_LEDGER_MIGRATION.md staged plan.

## Risk notes
RED — schema/migration + truth-ownership + append-only invariant. Human decision required. Produce an Escalation decision record (docs/ai/ESCALATION_PLAYBOOK.md); do not change semantics autonomously.

## Validation
N/A (audit). When approved, the staged PR carries characterization + migration tests.

## Done criteria
Decision record written; corrections-table design + re-derivation approach approved by a human before any code.

---

# DECISION RECORD (ARCHST-R1) — audit-only, no code/schema/ledger change

**Class:** E3 controlled-zone (append-only `action_ledger` truth ownership + schema/migration) — RED, human decision required.
**Trigger:** queue asks for the corrections-table design + re-derivation approach for the documented append-only UPDATE violations, ratified before any code.

## Verified current state (read-only, vs HEAD)

`action_ledger` is the append-only foundation of the Coordination Plane; `leads`
engagement projections read from it. `scripts/check_topology.sh [4]` confirms coordination
is the **sole `INSERT` writer** (ownership boundary intact). `[6]` tracks `LEDGER_UPDATE_BASELINE=3`
as **EXPECTED-FAIL** — i.e. 3 known in-place UPDATEs violate append-only and CI fails only if the
count *grows*. All three confirmed present at HEAD:

| # | Site (HEAD) | Shape | Caller | Tenant scope |
|---|-------------|-------|--------|--------------|
| 1 | `action_ledger.go:187` `MarkActionLedgerOutcome(id,outcome,reason)` | `UPDATE … SET outcome,reason WHERE id=?` | **No production caller** — test-only (`action_ledger_test.go:166`). | by-id only (no org filter) |
| 2 | `action_ledger.go:216` `MarkActionLedgerOutcomeByOutbound(org,outbound,…)` | `SELECT id … WHERE org_id=? AND outbound_id=? ORDER BY performed_at DESC LIMIT 1` then `UPDATE … WHERE id=?` | **HOT** — `server/agent/finalize_side_effects.go:143`, every verified outcome | org-scoped SELECT |
| 3 | `engagement_reconcile.go:165` `ReconcileEngagement` correction loop | `UPDATE … SET outcome='failed', reason='reconciled:…' WHERE id=? AND outcome='succeeded'` (CAS-guarded) | scheduled reconcile | org carried on `pendingFix` |

A complete design already exists: **`specs/store/APPEND_ONLY_LEDGER_MIGRATION.md`** (additive
`event_type` column on `action_ledger`; new append-only writers `RecordOutcomeForOutbound` /
`RecordEngagementRevoked`; new reader `CurrentOutcomeForOutbound`; staged PR A→B→C; risks +
acceptance criteria). This record **ratifies** that design and records the founder decision.

## Invariants the migration MUST preserve
- **Auditability:** the full attempt→outcome→correction history is retained; no row is
  overwritten or deleted — corrections are *new events* (`engagement_revoked`).
- **Idempotency:** re-running reconcile must not append duplicate `engagement_revoked` events
  (pre-check `SELECT 1 … WHERE outbound_id=? AND event_type='engagement_revoked'`, spec §4).
- **Tenant isolation:** every new writer is org-scoped. Violation #1's missing org filter must
  NOT be carried into the new API (`RecordOutcomeForOutbound` takes `orgID`).
- **No destructive rewrite/delete:** zero `UPDATE`/`DELETE action_ledger` at the end state
  (acceptance: topology `[6]` baseline 3 → 0, INSERT-only).
- **Projection equivalence:** event-derived "current outcome" must match today's column-derived
  outcome on historical data (replay test on a captured snapshot, spec §6).

## Options
- **A — Adopt the existing event-sourced-in-table plan (RECOMMENDED).** Implement
  `specs/store/APPEND_ONLY_LEDGER_MIGRATION.md` as staged PRs: **PR A** additive (`event_type`
  column default `action_attempted` + new writers/reader, old UPDATEs stay, baseline unchanged) →
  **PR B** reader migration (engagement projection reads `CurrentOutcomeForOutbound`, dual-write
  window) → **PR C** writer cutover (finalize + reconcile switch to append; remove `Mark*`;
  baseline 3→0). Each PR carries characterization + migration + replay tests; **each PR is a
  separate founder-gated RED change** — this record approves the *design + sequence*, not
  autonomous coding. Minimal new surface (one column, three methods), readers keep working
  throughout (expand/contract), reversible at every stage.
- **B — Separate `action_ledger_corrections` table.** A dedicated corrections/events table
  joined to `action_ledger` for projection. Rejected as default: adds a cross-table join to the
  HOT projection read, a second ownership surface, and a second migration, for no isolation gain
  over an additive `event_type` column on the table coordination already solely owns. Keep as the
  fallback only if a future requirement needs corrections physically separated (e.g. retention).
- **C — Defer; keep baseline 3 documented.** Status quo: topology `[6]` already prevents *new*
  violations, so the system is safe today. Rejected as the end state (leaves the append-only
  invariant violated and blocks correction events from non-reconcile sources), but acceptable as
  the *interim* until PR A is scheduled.

## Recommended default: **A**, with two scoping notes
1. **Quick win, separate GREEN item (not this audit):** violation #1 `MarkActionLedgerOutcome`
   (by-id) has **no production caller** — it can be deleted (or folded into the test) to drop the
   baseline 3→2 *before* the event-sourcing work, shrinking the cutover. File as a small GREEN
   follow-up; do not bundle into a RED PR.
2. **Sequence stays additive-first** per [[feedback_staged_evolution_over_big_bang]] and
   [[feedback_append_only_correction_events]] (reconcile emits `engagement_revoked`, never mutates).

## Why safe / remaining risk
Audit-only: **no production code, schema, migration, or ledger semantics changed.** Behavior
preserved by construction. Remaining risk lives in the *future* PRs (dual-write divergence in
PR B; `MAX(id)` projection cost) and is mitigated in spec §4 (CI divergence test; covering index;
optional materialised `last_event_id`). No PR proceeds without founder approval of that PR.

Item stays `REVIEW` for founder ratification of option A; DONE is set only by queue reconcile
after merge. Approving this unblocks `specs/store/APPEND_ONLY_LEDGER_MIGRATION.md` PR A.
