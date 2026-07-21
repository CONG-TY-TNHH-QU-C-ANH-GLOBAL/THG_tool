# Outbound Actions — V2 Refactor Staged Plan

Layer: **implementation** for the `outbound-actions` feature. Extracted
verbatim from the V2 Outbound Refactor design doc (§0, §1, §2.2, §6–§11;
authority: [../technical.md](../technical.md)) during the spec IA completion
sprint. PR1 shipped; PR2 remains staged.

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

## 2.2 Bảng `outbound_messages` — UNCHANGED in PR1

User chọn **Q5 = defer** (don't drop status until ext/contracts verified).

PR1: zero column changes on outbound_messages. We CONTINUE writing legacy `status` via `LegacyStatusFor` (already exists from PR-1). The plan to drop it stays in PR2.

PR1 just ensures all writes ALSO append a transition row.

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
