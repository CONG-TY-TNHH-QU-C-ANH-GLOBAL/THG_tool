# Outbound Actions — Technical Contract (V2 Outbound Refactor)

**Phiên bản**: v2 — revised per user feedback 2026-05-20 (staged evolution).
**Trạng thái**: APPROVED scope; PR1 shipped (policy-driven dedup in
`internal/store/outbound/dedup.go` + `action_policies` seeds); PR2 cleanup
remains staged in [implementation/refactor-plan.md](implementation/refactor-plan.md).
**Mandate**: `.claude/V2 Architecture Refactor and Tenant Isolation Standards.md`
**Liên kết**: [[feedback-verified-state-centric]] · [[feedback_append_only_correction_events]] · [[feedback_v2_tenant_isolation_mandates]] · [[feedback_staged_evolution_over_big_bang]]

The staged migration plan (revision history, PR1/PR2 sequencing, risks,
acceptance criteria, resolved open questions, non-goals) lives in
[implementation/refactor-plan.md](implementation/refactor-plan.md). This file
owns the stable technical invariants.

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
