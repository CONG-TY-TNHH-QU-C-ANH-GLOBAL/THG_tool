# V2 Outbound Refactor — Design Doc

**Phiên bản**: v2 — revised per user feedback 2026-05-20 (staged evolution).
**Trạng thái**: APPROVED scope. Sẵn sàng start PR1.
**Mandate**: `.claude/V2 Architecture Refactor and Tenant Isolation Standards.md`
**Liên kết**: [[feedback-verified-state-centric]] · [[feedback_append_only_correction_events]] · [[feedback_v2_tenant_isolation_mandates]] · [[feedback_staged_evolution_over_big_bang]]

---

## 0. Revision summary (what changed in v2)

User pushed back trên 4 điểm khi review v1, kết quả:

| v1 plan | v2 plan | Lý do |
|---------|---------|-------|
| Migrate to ledger-level CAS (`UNIQUE(outbound_id, attempt)` as primary CAS primitive) | **KEEP row-level CAS** (`UPDATE ... WHERE execution_state='planned'`). Append transition row trong same tx như side-effect, KHÔNG thay thế CAS | Row-level CAS đang stable + simple. Ledger CAS adds blast radius without proportional benefit at current maturity stage. |
| Include `revoke` / correction events | **DEFER** revoke semantics tới reconciliation work sau | Full event-sourcing purity overkill cho stage hiện tại. Reliable execution + tenant isolation + dedup correctness quan trọng hơn. |
| Drop legacy `status` column trong cùng refactor | **STAGED**: (1) stop writing → (2) stop reading → (3) verify ext/FE/webhook contracts → (4) drop. PR1 không drop column. | Drop column + state machine rewrite đồng thời = nightmare debug nếu drift xảy ra. |
| Bundle 5 phases vào 1 PR (~1500 LOC) | **2 PRs**: PR1 (ledger + isolation + policy) → PR2 (cleanup legacy + column drops + file split) | Smaller blast radius. PR1 = additive (lower risk). PR2 = breaking (after PR1 verified). |

**Triết lý mới (binding)**: staged evolution > big-bang refactor. Additive change đầu, breaking cleanup sau khi additive được verified production-stable.

---

## 1. Triết lý binding (5 directive từ V2 standards)

Mục tiêu cuối không đổi — phân bố qua 2 PRs:

| # | Directive | PR1 | PR2 |
|---|-----------|-----|-----|
| **D1** | Absolute Tenant Isolation | ✅ Delete 7 global functions, add CI linter | — |
| **D2** | Append-Only Ledger | ✅ Add transition rows trong same tx (additive). Row-level CAS unchanged. | — (no revoke events yet) |
| **D3** | Eradicate Legacy Tech Debt | ✅ Remove `requestedAuto` arg + `outbound_mode` policy key | ✅ Drop `status` column + `LegacyStatusFor()` |
| **D4** | Remove Hardcoded Coordination | ✅ `action_policies` table + policy-driven dedup | — |
| **D5** | Anti-God-Class + Design-First + CAS Atomicity | Design doc ✅ này. CAS atomicity = unchanged (row-level still works). | ✅ Split outbound.go into 8 files |

---

## 2. Schema thay đổi

### 2.1 Bảng `execution_attempts` — extend (KEEP NAME, no rename)

User chọn **Q1 = B** (keep name → less migration risk).

```sql
-- Migration: bump schemaBootstrapVersion = 5

-- Add transition_type column. Old rows (outcome-only) default to 'finalize'
-- so the row's meaning is preserved.
ALTER TABLE execution_attempts ADD COLUMN transition_type TEXT NOT NULL DEFAULT 'finalize';
-- Values: 'plan' | 'claim' | 'finalize' | 'reset'  (NO 'revoke' yet — defer)

-- Track which executor claimed (carries the CAS token through the ledger
-- for full audit trail, even though row-level CAS on outbound_messages
-- remains the authoritative concurrency control).
ALTER TABLE execution_attempts ADD COLUMN execution_id TEXT NOT NULL DEFAULT '';

-- Denormalize the resulting state pair so projection queries don't
-- need to interpret outcome strings.
ALTER TABLE execution_attempts ADD COLUMN resulting_state TEXT NOT NULL DEFAULT '';
ALTER TABLE execution_attempts ADD COLUMN resulting_outcome TEXT;

-- Lease window for executing transitions (read-only mirror — the
-- authoritative lease is still outbound_messages.lease_expiry).
ALTER TABLE execution_attempts ADD COLUMN lease_expiry DATETIME;

-- Index for "what was the latest transition for this outbound" queries.
CREATE INDEX IF NOT EXISTS idx_execution_attempts_latest
  ON execution_attempts(outbound_id, started_at DESC);

-- KHÔNG add UNIQUE(outbound_id, attempt) — row-level CAS on outbound_messages
-- is the concurrency primitive. The ledger is an additive audit trail.
-- (User feedback: don't migrate CAS primitive in same wave as state machine rewrite.)
```

**Why no UNIQUE constraint on (outbound_id, attempt)**: row-level UPDATE CAS on `outbound_messages.execution_state` is still the primary atomicity mechanism. Adding UNIQUE on the ledger would create a SECOND CAS surface → 2 sources of truth → race condition matrix doubles. Single CAS source = simpler reasoning.

### 2.2 Bảng `outbound_messages` — UNCHANGED in PR1

User chọn **Q5 = defer** (don't drop status until ext/contracts verified).

PR1: zero column changes on outbound_messages. We CONTINUE writing legacy `status` via `LegacyStatusFor` (already exists from PR-1). The plan to drop it stays in PR2.

PR1 just ensures all writes ALSO append a transition row.

### 2.3 Bảng `action_policies` — domain-agnostic coordination

User chọn **Q3 = A** (seed defaults). Schema unchanged from v1:

```sql
CREATE TABLE action_policies (
  id                   INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id               INTEGER NOT NULL,                  -- 0 = global default
  action_type          TEXT    NOT NULL,
  dedup_scope          TEXT    NOT NULL,                  -- 'per_account' | 'workspace' | 'none'
  block_on_planned     INTEGER NOT NULL DEFAULT 0,
  block_on_executing   INTEGER NOT NULL DEFAULT 1,
  cooldown_seconds     INTEGER NOT NULL DEFAULT 86400,
  conversation_aware   INTEGER NOT NULL DEFAULT 0,
  created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at           DATETIME,
  UNIQUE(org_id, action_type)
);

-- Seed defaults (replicates current hardcoded behaviour):
INSERT INTO action_policies (org_id, action_type, dedup_scope, block_on_planned, block_on_executing, cooldown_seconds, conversation_aware) VALUES
  (0, 'comment',      'per_account', 1, 1, 86400, 0),
  (0, 'inbox',        'workspace',   0, 1, 86400, 1),
  (0, 'group_post',   'per_account', 0, 1, 86400, 0),
  (0, 'profile_post', 'per_account', 0, 1, 86400, 0);
```

**Resolution rule**: `GetActionPolicy(orgID, actionType)` returns org-specific row if exists, else global default (org_id=0).

---

## 3. State machine — row-level CAS + additive ledger

### 3.1 Transition types (PR1)

| transition_type | When emitted | resulting_state | resulting_outcome | Concurrency control |
|-----------------|-------------|-----------------|-------------------|---------------------|
| `plan`     | QueueOutboundForOrg INSERT | planned   | NULL | Unique-on-active-target index (existing) |
| `claim`    | ClaimPlannedOutbound       | executing | NULL | Row-level CAS on outbound_messages.execution_state='planned' (UNCHANGED from PR-1) |
| `finalize` | FinalizeOutboundAttempt    | finished  | verified_success / context_drift / blocked / etc. | Row-level CAS on outbound_messages.execution_state='executing' + execution_id match (UNCHANGED) |
| `reset`    | ResetStaleExecuting        | planned   | NULL | Row-level UPDATE on outbound_messages with lease check (UNCHANGED) |

**Deferred to PR3+**: `revoke` correction events (when reconciliation lands).

### 3.2 Write pattern — additive transition row

```go
// Every state-changing function does (PSEUDOCODE):
//
//   1) BEGIN TX
//   2) UPDATE outbound_messages SET execution_state=?, verification_outcome=?, ...
//      WHERE id=? AND org_id=? AND execution_state=<expected>
//      → If RowsAffected == 0: ROLLBACK + return ErrRaceLost (row was claimed
//        by another worker / already terminal / wrong tenant)
//   3) INSERT INTO execution_attempts (outbound_id, org_id, account_id, attempt,
//      transition_type, resulting_state, resulting_outcome, execution_id,
//      lease_expiry, started_at, ...)
//      VALUES (..., <next_attempt_n>, <type>, <new_state>, <new_outcome>, ...)
//      → Best-effort. Ledger failure does NOT roll back the outbound update —
//        the audit trail is non-load-bearing for correctness, only for analytics.
//   4) COMMIT
//
// Row-level CAS at step 2 remains authoritative. Step 3 is additive ledger
// data for analytics / reconciliation / future event sourcing migration.
```

**Critical property**: PR1 ledger writes are **best-effort, not load-bearing**. If the INSERT fails (disk full, etc), the state transition still committed via step 2. The ledger row will be back-filled lazily by the reconciler (PR-2 of original engagement plan).

### 3.3 Why row-level CAS stays

| Concern | v1 (ledger CAS) | v2 (row CAS) |
|---------|----------------|--------------|
| Concurrency safety | UNIQUE on ledger + retry loop | UPDATE ... WHERE clause atomically — single statement |
| Single source of truth | Ledger + projection | outbound_messages row |
| Failure mode | INSERT collision → retry loop | RowsAffected == 0 → done |
| Code surface | New helper + retry logic + race tests | Existing CAS code unchanged |
| SQLite quirks | UNIQUE + concurrent INSERT contention | Well-understood, battle-tested |
| Future event-sourcing migration | Already done | Migration path exists, deferred |

User assessment confirmed: row-level CAS đủ mạnh cho current architecture. Ledger is additive — when reconciler lands, it consumes the audit trail. No correctness penalty for keeping row CAS now.

---

## 4. Tenant isolation — public API (PR1 scope)

### 4.1 Functions DELETED entirely

Per D1, no replacement — callers MUST switch to ForOrg variants. These 7 functions are confirmed dead (audit found no callers outside tests):

```
GetOutbound(id)                            → DELETED
GetOutboundByStatus(status, limit)         → DELETED
GetOutboundByFilter(status, msgType, limit) → DELETED
GetSentGroupPosts(withinDays)              → DELETED
DeleteOutbound(id)                         → DELETED
CountOutboundByStatus()                    → DELETED
UpdateOutboundStatus(id, status)           → DELETED
UpdateOutboundContent(id, content)         → DELETED
```

Tests that touch these will be migrated to `ForOrg` variants — estimated ~6 test cases.

### 4.2 Public API surface (PR1, additive only)

ALL EXISTING `ForOrg` functions kept with same signatures. ZERO breaking changes for callers in PR1 — just removing the global variants they shouldn't have been calling anyway.

**Renames deferred to PR2** (per "no rename + cleanup in same wave"):
- `ClaimApprovedOutboundForOrg` keeps name in PR1 (rename to `ClaimPlannedOutboundForOrg` in PR2)
- `ResetStaleSendingOutboundForOrg` keeps name (rename to `ResetStaleExecutingForOrg` in PR2)
- `FinalizeOutboundAttempt` already accepts orgID — no rename needed

**Q4 = OK**: `requestedAuto` removed from `QueueOutboundForOrg` signature. 2 callsites updated.

### 4.3 Linter gate

CI check `scripts/check_tenant_isolation.sh`:
- Fail if any `SELECT/UPDATE/DELETE ... FROM outbound_messages` lacks `org_id = ?` in WHERE.
- Fail if any new public store function lacks `orgID int64` in signature.

Whitelist exceptions only for migrate() in schema.go (DDL doesn't filter tenant) and reconciler functions that explicitly span orgs (must have audit log).

---

## 5. Domain-agnostic coordination (PR1)

### 5.1 Current hardcoded logic (to remove)

```go
// canQueueOutboundTx + CanQueueOutboundForOrg
crossAccount := msgType == "inbox"
if msgType == "comment" || execState == "planned" || execState == "executing" { ... }
```

### 5.2 Policy-driven replacement

```go
// internal/store/action_policy.go (NEW)

type ActionPolicy struct {
    OrgID              int64
    ActionType         string
    DedupScope         string  // 'per_account' | 'workspace' | 'none'
    BlockOnPlanned     bool
    BlockOnExecuting   bool
    CooldownSeconds    int
    ConversationAware  bool
}

// GetActionPolicy returns the effective policy: org-specific row if exists,
// else global default (org_id=0). Both queried in one statement via UNION.
func (s *Store) GetActionPolicy(ctx context.Context, orgID int64, actionType string) (*ActionPolicy, error)

// UpsertActionPolicy for admin override.
func (s *Store) UpsertActionPolicy(ctx context.Context, p ActionPolicy) error
```

```go
// internal/store/outbound_dedup.go (NEW — extract from outbound.go)

func (s *Store) CheckDedupTx(ctx context.Context, tx *sql.Tx, orgID, accountID int64,
    actionType, targetURL, profileURL string) (DedupDecision, error) {
    
    policy, err := s.GetActionPolicyTx(ctx, tx, orgID, actionType)
    if err != nil { return DedupDecision{}, err }
    
    // SQL built from policy.DedupScope ('per_account' adds account_id filter, etc).
    // Block predicates read from policy fields.
    // Conversation gate runs only when policy.ConversationAware.
    ...
}

// Behaviour profile caps applied separately (existing checkBehaviourCapsTx
// stays as-is — it's already domain-agnostic).
```

`canQueueOutboundTx` and `CanQueueOutboundForOrg` both delegate to `CheckDedupTx` + `checkBehaviourCapsTx`. Bug fix: `CanQueueOutboundForOrg` (non-tx) currently skips behaviour caps — now both paths call the same gate functions.

---

## 6. File split — DEFERRED to PR2

PR1 keeps `outbound.go` as-is (973 LOC). The split into 8 files happens in PR2 after:
- Legacy `status` column dropped
- `LegacyStatusFor` removed
- Rename functions to V2 lexicon (`ClaimPlanned*`, `ResetStaleExecuting*`)

Reason: doing both rewrite + split in one PR makes review impossible. PR2 is pure code organization once the semantic ground is stable.

---

## 7. Migration plan — 2 PRs

### PR1 — Additive (lower risk)

**Goal**: introduce transition ledger + tenant isolation + policy system. ZERO drops, ZERO breaking renames.

1. Schema migration v5:
   - Add columns to `execution_attempts` (transition_type, execution_id, resulting_state, resulting_outcome, lease_expiry)
   - Add `action_policies` table + seed 4 defaults
2. Refactor write path:
   - `Claim`, `Finalize`, `Reset` each append a transition row in same tx as the existing row-level CAS UPDATE
   - One shared helper `appendOutboundTransition(tx, transitionRow)` — best-effort
3. Delete 7 global functions (D1)
4. Replace hardcoded msgType logic with `CheckDedupTx` policy lookup (D4)
5. Remove `requestedAuto` from QueueOutboundForOrg signature
6. Add CI linter `check_tenant_isolation.sh`
7. Migrate ~6 test cases that used deleted globals
8. **NO column drops, NO renames, NO file split, NO revoke events**

**Estimated change**: ~400 LOC across 6 files. Mostly additive. Tests pass with row CAS unchanged.

### PR2 — Breaking cleanup (after PR1 production-verified)

**Goal**: drop legacy artifacts after PR1 has shown additive ledger writes work correctly.

Prereqs: PR1 deployed, at least 1 week of production traffic, reconciliation job (from original PR-2 plan) verifies transition ledger matches outbound_messages state.

1. Stop writing legacy `status` column (silent no-op write for 1 cycle)
2. Stop reading legacy `status` everywhere in BE (engagement queries, dashboards)
3. Verify extension + webhook contracts: extension doesn't read `status` back; webhook payloads use `execution_state`/`verification_outcome`
4. Drop columns from outbound_messages: `status`, `claimed_by`, `claimed_at`, `execution_id`, `lease_expiry`, `sent_at` (move state to projection columns + execution_attempts)
5. Remove `models.LegacyStatusFor()` and 6 remaining legacy `OutboundStatus` constants
6. Rename functions to V2 lexicon: `ClaimPlannedOutbound*`, `ResetStaleExecuting*`
7. Split `outbound.go` into 8 focused files (avg ~110 LOC each)

**Estimated change**: ~800 LOC across 15 files. Breaking — needs feature flag rollout or coordinated deploy.

---

## 8. Risks + mitigations (PR1)

| Risk | Severity | Mitigation |
|------|----------|------------|
| Transition ledger write fails (disk full, lock contention) | Low | Best-effort INSERT, errors logged not propagated. Row-level CAS already committed. Reconciler back-fills missing rows. |
| Policy lookup adds latency to enqueue path | Low | Single SELECT per enqueue. Indexed UNIQUE(org_id, action_type). Cache later if needed. |
| Seed defaults conflict with existing custom logic | Low | Default values mirror current hardcoded behaviour exactly — A/B compare test included. |
| Migration v5 fails on production DB | Medium | All ALTER statements idempotent (`ADD COLUMN`, `CREATE INDEX IF NOT EXISTS`). Roll-back: drop new columns is no-op safe. |
| Linter false positives on existing helpers | Low | Whitelist exceptions documented + tested. Initial run audited manually. |
| Test fixtures use deleted globals | Low | ~6 test cases identified in audit, easy migration to ForOrg. |

PR2 risks (drop column, rename, split) handled separately when that PR lands.

---

## 9. Acceptance criteria (PR1)

Before PR1 merge:

- [ ] `scripts/check_tenant_isolation.sh` returns 0 violations.
- [ ] 7 global functions deleted: GetOutbound, GetOutboundByStatus, GetOutboundByFilter, GetSentGroupPosts, DeleteOutbound, CountOutboundByStatus, UpdateOutboundStatus.
- [ ] `execution_attempts` table has columns: transition_type, execution_id, resulting_state, resulting_outcome, lease_expiry.
- [ ] `action_policies` table exists with 4 seeded defaults.
- [ ] No `msgType == "inbox"` or `msgType == "comment"` literals remain in `internal/store/`.
- [ ] Every state transition (Claim, Finalize, Reset) writes a transition row in same tx.
- [ ] `requestedAuto` removed from QueueOutboundForOrg signature; 2 callsites updated.
- [ ] `CanQueueOutboundForOrg` (non-tx) now ALSO runs behaviour caps check (bug fix carried in this PR).
- [ ] Row-level CAS tests still pass (TestFinalize*_FirstWin, _IdempotentReplay, _StaleExecutionID, _LegacyEmptyToken).
- [ ] New test: assert transition row appended for each Claim/Finalize/Reset.
- [ ] New test: assert policy-driven dedup matches old hardcoded behaviour for 4 default action types.
- [ ] `go test ./...` green.
- [ ] `go vet ./...` green.
- [ ] `npm --prefix frontend run build` green.

PR1 does NOT require:
- Column drops
- Renames
- File splits
- Revoke event support

---

## 10. Open questions — RESOLVED

| Q | Decision | Source |
|---|----------|--------|
| Q1 Table rename | **B — keep `execution_attempts` name** | User choice (less migration risk) |
| Q2 Projection columns | **A — keep execution_state + verification_outcome as cache** | User choice (denormalized for fast reads) |
| Q3 Action policy seeding | **A — seed 4 defaults at bootstrap** | User choice (compat) |
| Q4 requestedAuto removal | **YES — drop from signature** | User confirmed |
| Q5 Legacy status column drop | **DEFER to PR2** — stop writing/reading first, verify ext/FE contracts, then drop | User correction (don't drop with rewrite in same wave) |
| Q6 PR scope | **B — 2 PRs**: PR1 (ledger + isolation + policy), PR2 (cleanup + drops + rename + split) | User correction (smaller blast radius) |

---

## 11. Non-goals (rõ ràng OUT of scope of PR1)

- **KHÔNG** drop legacy `status` column (defer PR2)
- **KHÔNG** rename functions (defer PR2)
- **KHÔNG** split outbound.go into multiple files (defer PR2)
- **KHÔNG** introduce `revoke` correction events (defer to reconciliation work)
- **KHÔNG** migrate CAS primitive to ledger UNIQUE constraint (keep row CAS)
- **KHÔNG** refactor handler boilerplate (audit R-2, separate)
- **KHÔNG** refactor scan helpers for other tables (audit R-3, separate)
- **KHÔNG** wire Account Reputation projection into dedup (PR-2.5 plan, separate)
- **KHÔNG** drop `action_ledger` table (used by coordination plane, not state machine)
- **KHÔNG** change extension contract (extension still echoes execution_id + outcome)
- **KHÔNG** migrate other stores (leads.go, threads.go) — pattern applies later
