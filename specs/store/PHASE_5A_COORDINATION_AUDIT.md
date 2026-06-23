# Phase 5A — Coordination Extraction Audit

**Status**: deliverable for Phase 5A per [[project_coordination_phase_risks]] §B. ZERO code moves in this PR. Output is this document + a small set of `// tenant-ok` annotations.

**Gate**: Phase 5B may NOT open until (a) all BLOCKER findings in §4 are resolved by pre-5B PRs, and (b) this audit is reviewed + approved by user.

**Date**: 2026-05-21.

---

## §1. Scope

Audit the coordination domain (action_ledger, behaviour_profile, engagement_reconcile, execution_attempts, behaviour_caps_check) for extraction readiness. Produce 6 inventories. Surface blockers. Recommend pre-5B PRs.

References:
- Ownership: [DOMAINS.md §2.4](../internal/store/DOMAINS.md#24-truth-ownership-matrix-locked-2026-05-21)
- Strategy: [[project_coordination_phase_risks]] §B (5A/5B/5C)
- Subpackage contract: [[feedback_subpackage_contract]] (9 points)
- Anti-patterns: [[feedback_no_coordination_service]], [[feedback_no_bidirectional_domain_knowledge]], [[feedback_extraction_is_not_redesign]]

---

## §2. Public API inventory

14 exported methods on `*Store` across 5 files. 6 exported types. 3 unexported tx-helpers consumed via closure indirection from outbound's hook framework.

### action_ledger.go (4 methods, 2 types, 1 tx-helper)

| Method | Signature (abbreviated) | Peer-import params? | Tx? |
|--------|-------------------------|---------------------|-----|
| `RecordActionLedger` | `(ctx, entry ActionLedgerEntry) (int64, error)` | No (local struct) | No |
| `ListActionLedger` | `(ctx, orgID, actionType, targetURL, since, limit) ([]ActionLedgerEntry, error)` | No (primitives) | No |
| `MarkActionLedgerOutcome` | `(ctx, ledgerID, outcome, reason) error` | No | No |
| `MarkActionLedgerOutcomeByOutbound` | `(ctx, orgID, outboundID, outcome, reason) (int64, error)` | No | No |

Types: `ActionLedgerEntry` (struct), `LedgerOutcome*` constants. Both internal to this file; external readers can take a copy on extraction.

Tx-helper: `recordActionLedgerTx(tx, orgID, accountID, actionType, targetURL, outboundID, cooldown) error` — wired via outbound hook closure.

### behaviour_profile.go (6 methods, 0 types, 1 tx-helper)

| Method | Signature (abbreviated) | Peer-import params? | Tx? |
|--------|-------------------------|---------------------|-----|
| `GetAccountBehaviourProfile` | `(ctx, accountID) (*models.AccountBehaviourProfile, error)` | ⚠️ `models.*` (shared types layer — see §4.2) | No |
| `UpsertAccountBehaviourProfile` | `(ctx, *models.AccountBehaviourProfile) error` | ⚠️ same | No |
| `GetAccountRuntimeState` | `(ctx, accountID) (models.AccountRuntimeState, error)` | ⚠️ same | No |
| `ApplyRiskSignal` | `(ctx, orgID, accountID, models.RiskSignal, weight) error` | ⚠️ same | No |
| `SetAccountCooldown` | `(ctx, orgID, accountID, until) error` | No | No |
| `ResolveAccountCaps` | `(ctx, accountID) (models.BehaviourCaps, models.TrustLevel, error)` | ⚠️ same | No |

Tx-helper: `incrementRuntimeCounterTx(tx, orgID, accountID, action) error` — wired via outbound hook closure.

### engagement_reconcile.go (1 method, 1 type)

| Method | Signature | Peer-import params? | Tx? |
|--------|-----------|---------------------|-----|
| `ReconcileEngagement` | `(ctx, orgID) (*ReconcileEngagementReport, error)` | No | No |

Type: `ReconcileEngagementReport` (struct) — internal.

### execution_attempts.go (9 methods, 3 types)

| Method | Signature (abbreviated) | Peer-import params? | Tx? |
|--------|-------------------------|---------------------|-----|
| `BeginExecutionAttempt` | `(ctx, models.ExecutionAttempt) (int64, error)` | ⚠️ `models.*` | No |
| `FinishExecutionAttempt` | `(ctx, attemptID, models.ExecutionOutcome, failureReason, VerificationEvidence) error` | ⚠️ `models.*` + local | No |
| `AdvanceAttemptStatus` | `(ctx, attemptID, models.AttemptStatus) error` | ⚠️ `models.*` | No |
| `GetExecutionAttempt` | `(ctx, attemptID) (models.ExecutionAttempt, error)` | ⚠️ same | No |
| `ListAttemptsForOutbound` | `(ctx, outboundID) ([]models.ExecutionAttempt, error)` | ⚠️ same | No |
| `CountRecentAttemptsByAccount` | `(ctx, orgID, accountID, models.ExecutionOutcome, since) (int, error)` | ⚠️ same | No |
| `ExecutionOutcomeDistribution` | `(ctx, orgID, since) ([]OutcomeDistributionBucket, error)` | No (local struct return) | No |
| `ListRecentExecutionAttempts` | `(ctx, orgID, since, limit) ([]models.ExecutionAttempt, error)` | ⚠️ same | No |
| `AccountHealthSnapshot` | `(ctx, orgID, accountID) ([]AccountHealthRow, error)` | No (local struct return) | No |

Types: `VerificationEvidence`, `OutcomeDistributionBucket`, `AccountHealthRow` (structs) — internal.

### behaviour_caps_check.go (1 unexported tx-helper, FORBIDDEN return type)

| Helper | Signature | Issue |
|--------|-----------|-------|
| `checkBehaviourCapsTx` | `(tx, accountID, msgType) (outbound.GuardDecision, error)` | 🔴 **Returns outbound.GuardDecision** — coordination must NOT import outbound. See §4.1. |

---

## §3. Caller graph

Total production callers (non-test): **7**. Test callers: ~50. Per Phase 2 precedent (≤6 = clean-cut, 7-14 = bridge reasonable, ≥15 = bridge strongly), coordination sits at the edge.

### Production call sites

| Method | Production caller | Site |
|--------|-------------------|------|
| `BeginExecutionAttempt` | internal/server/agent | outbox_agent.go:283 (finalizeOutbound) |
| `FinishExecutionAttempt` | internal/server/agent | outbox_agent.go:304 (finalizeOutbound) |
| `MarkActionLedgerOutcomeByOutbound` | internal/server/agent | outbox_agent.go:314 (finalizeOutbound) |
| `ApplyRiskSignal` | internal/server/agent | outbox_agent.go:320 (finalizeOutbound) |
| `ExecutionOutcomeDistribution` | internal/server/observability | handlers.go:36 |
| `ListRecentExecutionAttempts` | internal/server/observability | handlers.go:100 |
| `AccountHealthSnapshot` | internal/server/observability | handlers.go:161 |

Caller-package diversity: **2 packages** (server/agent for writes via finalizeOutbound; server/observability for dashboard reads).

### Tx-threaded helpers (closure-injected, never standalone)

| Helper | Wired into | Frequency |
|--------|------------|-----------|
| `recordActionLedgerTx` | `outbound.Hooks.RecordLedger` (outbound_aliases.go:102) | Every QueueOutboundForOrg call |
| `incrementRuntimeCounterTx` | `outbound.Hooks.IncrementCounter` (outbound_aliases.go:105) | Every QueueOutboundForOrg call |
| `checkBehaviourCapsTx` | `outbound.Hooks.BehaviourCheck` (outbound_aliases.go:95) | Every QueueOutboundForOrg call |

All three are registered ONCE at Store construction (`installOutboundHooks`) and consumed inside outbound's queue transaction. No standalone call sites.

---

## §4. BLOCKERS — must clear before 5B

### 4.1 🔴 Peer-import cycle: `outbound.GuardDecision` consumed by coordination

`behaviour_caps_check.go:33-67` declares:

```go
func (s *Store) checkBehaviourCapsTx(tx *sql.Tx, accountID int64, msgType string) (outbound.GuardDecision, error)
```

This violates [[feedback_no_bidirectional_domain_knowledge]]: coordination is BELOW outbound in the dependency graph; coordination must NOT import outbound types. The hook framework currently dodges this via closure indirection (outbound's hook receiver casts the type), but extraction makes the import explicit and breaks compilation.

**Recommendation**: pre-5B PR — split into primitive return:

```go
// New coordination-local type:
type CapsDecision struct {
    Allowed bool
    Reason  string  // "account_cooldown_active" | "daily_limit_exceeded" | "risk_ceiling_exceeded" | ""
}

func (s *Store) CheckBehaviourCaps(tx *sql.Tx, accountID int64, msgType string) (CapsDecision, error)
```

Then outbound's hook adapter (in outbound_aliases.go) converts `CapsDecision` → `outbound.GuardDecision` at the boundary. Pure mechanical refactor; tests pass with adapter updates only.

Effort: ~50 LOC. One pre-5B PR.

### 4.2 🔴 Cross-domain write: outbound writes execution_attempts directly

`internal/store/outbound/transition.go:87-103` INSERTs into `execution_attempts` (coordination-owned table). The annotation acknowledges this is interim:

```go
// tenant-ok: cross-domain projection (outbound -> coordination). The
// execution_attempts table is owned by the coordination domain
// (Phase 5 target). Outbound writes to it directly today as the
// append-only audit ledger — this will move to an injected hook when
// coordination is extracted.
```

This is the only cross-domain write touching coordination-owned tables from outside the domain (per §5 audit). After extraction, outbound importing `internal/store/coordination` to call its method would be an upward import — forbidden.

**Recommendation**: pre-5B PR — add `Hooks.RecordTransition` callback to `outbound.Hooks` struct. coordination installs the implementation at boot (same pattern as RecordLedger). Outbound calls `hooks.RecordTransition(...)` instead of writing SQL directly.

Effort: ~80 LOC. One pre-5B PR. Tests must verify the transition row is still written.

### 4.3 🟡 Missing `// tenant-ok` annotation: leads reads action_ledger

`internal/store/lead_engagement.go:265-271` SELECTs `action_ledger` with JOINs to `accounts` + `users` (computing the lead engagement projection). No annotation present.

**Recommendation**: add this annotation in 5A doc PR (this PR):

```go
// tenant-ok: cross-domain projection (leads -> coordination). Reads
// action_ledger as the source of engagement state. Per truth ownership
// matrix (DOMAINS.md §2.4), action_ledger is owned by coordination; this
// JOIN is read-only.
```

Effort: 1 line. Bundle with this audit doc.

---

## §5. Semantic finding — NOT a 5B blocker, but document

### 5.1 Append-only invariant vs. observed UPDATEs

Memory [[feedback_append_only_correction_events]] states: "Ledger immutable. Reconciliation emits engagement_revoked events, never UPDATE/DELETE."

Audit found **5 UPDATE sites** on coordination's authoritative tables:

| Site | Statement |
|------|-----------|
| action_ledger.go:188 | `UPDATE action_ledger SET outcome = ?, reason = ? WHERE id = ?` (MarkActionLedgerOutcome) |
| action_ledger.go:229 | `UPDATE action_ledger SET outcome = ?, reason = ? WHERE id = ? AND ...` (MarkActionLedgerOutcomeByOutbound) |
| engagement_reconcile.go:165 | `UPDATE action_ledger SET outcome = ?, reason = ? WHERE id = ? AND outcome = ?` (reconciler downgrade) |
| execution_attempts.go:119 | `UPDATE execution_attempts SET ... outcome = ?, ...` (FinishExecutionAttempt — terminal commit) |
| execution_attempts.go:151 | `UPDATE execution_attempts SET status = ?` (AdvanceAttemptStatus) |

**Interpretation**: the code's effective invariant is **"row identity is immutable; outcome/status columns are soft-update-once-at-terminal"**. Reconcile's UPDATE specifically only downgrades `succeeded` → `failed` (with `WHERE outcome = 'succeeded'` guard), which is semantically a correction-event delivered as an UPDATE rather than an INSERT.

This is a tension between aspirational invariant (memory) and shipped reality (code).

**Recommendation**: per [[feedback_extraction_is_not_redesign]], do NOT refactor in 5B. Two options:

- **Option A (defer)**: leave UPDATEs as-is. Update [[feedback_append_only_correction_events]] memory to clarify: "row insertion is append-only; outcome/status terminal-write columns are soft. New correction events are NEW rows."
- **Option B (true append-only)**: change reconcile + Mark* methods to INSERT new rows with `event_type='revocation'` or similar. Requires schema add (`event_type` column) and projection updates everywhere that reads `outcome`. Pre-release timing favors this, but it is a **semantic redesign** that needs its own PR (and its own design-doc decision).

**For 5A purposes**: defer the choice. Phase 5B ships with current semantics intact. After 5B + production soak, decide A vs B in a dedicated PR.

---

## §6. Green lights

- ✅ **Caller fan-out**: 7 production sites across 2 packages → clean-cut viable
- ✅ **No interfaces**: zero interface declarations in coordination files (L4 compliant)
- ✅ **No forbidden names**: zero `Engine`, `Manager`, `Service`, `Dispatcher`, `Coordinator`, `Orchestrator` (per [[feedback_no_coordination_service]])
- ✅ **No cross-domain writes from inside coordination files**: every INSERT/UPDATE in coordination files targets only coordination-owned tables
- ✅ **Tx threading already cross-domain via hooks**: 3 tx-helpers consumed via closure injection — no new mechanism needed
- ✅ **`models.*` type usage is acceptable**: `internal/models` is a shared-types/DTO layer (below coordination in graph), not a peer domain. Compare to forbidden case in §4.1 where `outbound.*` is a peer.

---

## §7. L2 decision: CLEAN-CUT

Matches Phase 4 knowledge precedent. Justification:

- ≤2 production callers per method (most are 1)
- All production callers are in package layers that can be migrated atomically with the extraction (server/agent, server/observability)
- Hook framework already isolates the tx-threaded helpers; no new bridge needed for them
- Bridge wrappers would add `outbound_aliases.go`-style technical debt that L2 lock says must expire

→ **No top-level bridge methods**. All 14 callers (production + test) migrate to `s.Coordination().Foo()` directly.

---

## §8. Recommended pre-5B PR sequence

Five PRs in this order. Each ships independently and is reviewed independently:

| Order | PR | What it does | Effort |
|-------|-----|--------------|--------|
| 1 | **5A doc PR** (this PR) | Lands this audit + 1 annotation in lead_engagement.go:265 | <100 LOC |
| 2 | **Decouple-1** | Replace `outbound.GuardDecision` return with primitive `CapsDecision` in coordination | ~50 LOC |
| 3 | **Decouple-2** | Add `Hooks.RecordTransition` callback; move outbound/transition.go's direct INSERT through hook | ~80 LOC |
| 4 | **5B mechanical extraction** | Move 5 files → `internal/store/coordination/`. Migrate callers. ZERO semantic changes. | ~600 LOC moved + ~100 caller-site updates |
| 5 | **5C cleanup** (optional, after ≥1 week production soak) | Naming, dedup, append-only semantic decision (§5.1 A or B) | variable |

PR 2 and PR 3 are independent — can ship in either order. Both must land before PR 4 opens.

---

## §9. Phase 5B scope (locked once §8 PRs 1-3 land)

When 5B opens, the diff will:

1. Create `internal/store/coordination/store.go` (Store + NewStore + dialect wrappers, mirrors knowledge/store.go)
2. Create `internal/store/coordination/testing_helpers_test.go` (5-line storetest binding)
3. Move 5 files (action_ledger.go, behaviour_profile.go, engagement_reconcile.go, execution_attempts.go, behaviour_caps_check.go) → coordination/
4. Move associated test files into coordination/ as `package coordination_test`
5. Add `s.coordination *coordination.Store` field + `Coordination() *coordination.Store` accessor in top-level store.go
6. Update `installOutboundHooks` to wire hooks against `s.coordination` (instead of unexported helpers)
7. Migrate 7 production caller sites to `s.Coordination().XxX()`
8. Migrate ~50 test caller sites
9. Drop now-empty `behaviour_caps_check.go` (helper renamed + relocated)
10. Update DOMAINS.md row + spec doc Migration Log entry

**ZERO semantic changes**. Every UPDATE/INSERT/SELECT statement byte-identical to pre-extraction. Tests prove behavior unchanged.

---

## §10. Phase 5B merge gate (locked, references)

Identical to [[project_coordination_phase_risks]] §D — 8 items. Re-stated here for self-containment:

- [ ] Outbound → coordination calls use primitive args (no peer types). Verified by §8 PR 2.
- [ ] Tx-threading audit (§3) still accurate; PR description re-lists every cross-package `*sql.Tx` site.
- [ ] Coordination API has zero read-back methods that "tell" outbound its own state.
- [ ] No new interfaces in coordination/ (subpackage contract rule 6).
- [ ] All reconcile SQL is INSERT-only OR §5.1 Option A documented (memory updated). `grep "UPDATE action_ledger\|DELETE.*action_ledger" internal/store/coordination/` shows expected count only.
- [ ] No bidirectional knowledge: `grep "internal/store/outbound\|internal/store/leads" internal/store/coordination/` empty.
- [ ] No forbidden names: `grep -E "(CoordinationEngine|CoordinationManager|CoordinationService|RuntimeService|ExecutionCoordinator|EventDispatcher|LedgerOrchestrator)" internal/store/coordination/` empty.
- [ ] Subpackage contract (DOMAINS.md §3, 9 points) all satisfied.

---

## §11. Outstanding decisions for user

Before 5B opens, please confirm:

1. **§4.1 fix approach**: introduce `coordination.CapsDecision` primitive struct (recommended) vs. move `outbound.GuardDecision` to a shared types layer?
2. **§4.2 fix approach**: `Hooks.RecordTransition` callback (recommended, matches existing hook pattern) vs. some other indirection?
3. **§5.1 append-only**: defer to 5C (Option A: update memory) or pre-extraction (Option B: refactor to true append-only)? Recommendation: **Option A**.

These three decisions gate the pre-5B PR sequence in §8.

---

## §12. Appendix — audit method

This audit was produced by three parallel investigations:
1. Public API inventory — read every coordination file, extract method signatures + types
2. Caller graph — grep every method name across the repo
3. Cross-domain SQL inventory — scan for SQL statements crossing coordination/non-coordination boundaries

Plus targeted grep for append-only invariant (`UPDATE action_ledger`, `UPDATE execution_attempts`).

Re-running the audit before 5B opens: same three queries, expect deltas only at §4 sites (after pre-5B PRs land).
