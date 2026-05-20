# `internal/store/` — Subpackage Refactor Design Doc

**Trạng thái**: APPROVED scope. Phase 0 + Phase 1 in same PR; Phase 2 separate PR.
**Ngày**: 2026-05-21
**Mandate**: User yêu cầu ultrathink chia subfolder để codebase scale dễ hơn.
**Liên kết**: [[feedback_staged_evolution_over_big_bang]] · [[feedback_v2_tenant_isolation_mandates]] · [[feedback_freeze_abstraction]] · [[feedback-store-subpackage-locks]]

---

## 0. Locked invariants (binding for every phase)

User added 4 mandatory rules on approval — these apply to **every** subpackage extraction now and in the future:

### L1 — Dependency direction (no cycles)
Subpackages must NOT create cyclic dependencies. Dependency direction must remain **foundational → upward** only.
- `dbutil/` is the lowest layer; everything can import it
- Domain packages can import `dbutil/` + foundational domains (users, coordination)
- Domain packages must NOT import each other except in the documented foundational → upward direction
- Go compiler catches cycles, but the design must not require them

### L2 — Bridge-wrapper debt has expiry
Legacy top-level wrappers (`store.Store.QueueOutboundForOrg(...)` etc.) are **compatibility shims only**. They exist to avoid breaking the 14+ caller migration. **NEW code after Phase 2 ships MUST import subpackages directly** — no new top-level `store.Store` methods may be added that just delegate to a subpackage. CI / code review enforces this.

### L3 — Cross-package transactions thread the same `*sql.Tx`
Cross-package write operations must thread the **same `*sql.Tx`** through the call chain. Subpackages MUST NOT silently open nested transactions. Convention: subpackage methods that participate in a parent tx accept `tx *sql.Tx` as an arg (or accept a context that carries the tx). Subpackages that need their own tx for read-side queries must document it.

### L4 — No abstraction theater
Do NOT introduce repository interfaces, mock factories, or DI containers during this extraction. Preserve concrete `*Store` composition unless a real package boundary genuinely requires interface seams. Aligns with [[feedback_freeze_abstraction]]: the goal is encapsulation, not architecture astronautics.

---

## 0. TL;DR

`internal/store/` đã thành god-package: **89 files, 17,334 LOC, 308 public `*Store` methods, 57 files trong 1 package**. Subpackaging là hướng đúng cho scale, nhưng cost thực:

- **308 caller-side method renames** nếu đi pattern `s.Outbound.Queue()` thay vì `s.QueueOutboundForOrg()`
- **61 helper callsites** (parseSQLiteTime, retryOnBusy, utcDayKey, boolToInt) cross-cuts mọi file
- **Cross-domain SQL** (lead JOIN outbound, engagement reconcile spans 3 tables) cần thiết kế lại
- **Transactions across packages** (queue tx touches outbound + ledger + behaviour) cần `*sql.Tx` đi qua interface boundaries

→ Đề xuất **staged extraction, không big-bang**. Phase 1 (low risk) trước, các domain self-contained extract dần. Tuyệt đối không refactor leads/coordination/users trong wave này (entangled).

---

## 1. Hiện trạng

### 1.1 Inventory

| Cluster | Files | LOC | Cross-package coupling | Mức độ self-contained |
|---------|-------|-----|------------------------|-----------------------|
| **Outbound** (just refactored) | 12 | 2,155 | recordActionLedgerTx, incrementRuntimeCounterTx, ResolveAccountCaps, GetThreadByProfileForOrg | 7/10 |
| **Knowledge OS** | 15 | 3,553 | minimal — already partially in workspace_knowledge/ | 9/10 |
| **Coordination (ledger + behaviour + engagement)** | 8 | 2,055 | called by outbound, leads | 4/10 |
| **Leads (leads + engagement projection + niches)** | 5 | 1,497 | EXISTS-joins outbound, reads action_ledger | 3/10 |
| **Crawl (intents + posts + groups + sources)** | 6 | 1,297 | minimal | 8/10 |
| **Accounts + sessions + tokens + identities** | 7 | 1,067 | called by outbound (caps), agent pairing | 5/10 |
| **Connectors (commands, ownership, pairing, streams)** | 5 | 613 | called by accounts, outbound | 6/10 |
| **Prompts + routing + skills** | 4 | 990 | called by AI/orchestrator | 8/10 |
| **Threads / inbox** | 1 | 271 | called by outbound dedup | 6/10 |
| **Users + organization** | 2 | 506 | foundational, called by everything | 2/10 |
| **Infra (schema, store, migrator, dialect, sqlite)** | 8 | ~1,950 | core — must stay shared | N/A |
| **Misc (app, kpi, learning, media, price_items, etc.)** | 12 | ~1,000 | various | mixed |

### 1.2 Vấn đề thực tế

1. **Discoverability**: developer mới không biết "comment outbound logic nằm ở đâu" — phải grep.
2. **Merge conflict surface**: 1 package = mọi PR đụng `internal/store/` đều có risk conflict.
3. **API surface ambiguity**: `*Store` có 308 methods — không biết method nào thuộc domain nào.
4. **Cross-domain SQL trá hình tightly couples**: leads.go có `EXISTS(SELECT FROM outbound_messages...)` — đây là cross-domain dependency nằm trong SQL, invisible to imports.
5. **Refactor blast radius**: thay đổi 1 helper (vd `parseSQLiteTime`) ảnh hưởng 17 files.

---

## 2. Go-specific constraints (binding cho mọi quyết định)

Khác với Java/TypeScript, **Go folder = package**. Hậu quả:

| Action | Cost |
|--------|------|
| Move file vào subfolder | File trở thành package mới, mọi caller phải import + đổi method name |
| Method `s.appendOutboundTransition()` (unexported) cần gọi từ ngoài package | Phải export (`AppendOutboundTransition`) — leak internal API |
| `*sql.Tx` qua interface boundary | OK, nhưng helper functions (parseSQLiteTime) phải public hoặc duplicated |
| Cross-package transaction | Cần pass `*sql.Tx` qua package boundaries, mọi subpackage method nhận tx |
| `*Store` god-struct methods | Mỗi method giờ thuộc subpackage's struct → caller `s.QueueOutboundForOrg()` → `s.Outbound.Queue()` |

**Idiom check**: Go community split, nhưng codebases trên 15k LOC trong 1 package thường chuyển sang subpackages (Gitea models/, CockroachDB sub-stores). Flat package phù hợp < 10k LOC.

---

## 3. Bridge pattern — caller migration miễn phí

Có 1 pattern bypass được 308-callsite migration cost: **giữ Store wrapper, subpackage là implementation**.

```go
// internal/store/store.go (top level)
package store

import (
    "github.com/thg/scraper/internal/store/outbound"
    "github.com/thg/scraper/internal/store/crawl"
    ...
)

type Store struct {
    db *sql.DB

    outbound *outbound.Store   // composition
    crawl    *crawl.Store
    ...
}

// Thin delegate — old caller signature preserved.
func (s *Store) QueueOutboundForOrg(msg *models.OutboundMessage, cooldown time.Duration) (OutboundQueueResult, error) {
    return s.outbound.Queue(msg, cooldown)
}
```

**Pros**:
- Zero caller migration in PR-1
- New code can opt into clean `s.outbound.Queue(...)` path
- Wrappers can be deleted gradually over future PRs

**Cons**:
- ~30-40 delegate functions of boilerplate per extracted domain
- Risk: developers don't migrate, wrappers stay forever
- Types now duplicated (`store.OutboundQueueResult` re-exports `outbound.QueueResult`)

**Đề xuất**: dùng bridge pattern CHO outbound (vì callers nhiều), KHÔNG dùng cho domain mới (templates/telegram/copilot — start clean).

---

## 4. Đề xuất target structure (cuối cùng, không phải làm liền)

```
internal/store/
  store.go              <- Store struct (composition root), NewStore(db)
  schema.go             <- migrate(), schemaBootstrapVersion (could split → see §5.5)
  migrator.go           <- file migration framework
  backup.go             <- DB backup ops
  stats.go              <- dashboard stats
  
  dbutil/               <- shared helpers (extracted Phase 1)
    time.go             <- parseSQLiteTime, utcDayKey
    retry.go            <- retryOnBusy
    conv.go             <- boolToInt
    dialect.go + dialect_sqlite.go + dialect_postgres.go + postgres_driver.go + sqlite.go
  
  outbound/             <- Phase 2 (just refactored, clean seam)
    queue.go, query.go, claim.go, finalize.go, lease.go, edit.go, dedup.go, transition.go, policy.go
    types.go            <- QueueResult, GuardDecision, ClaimResult, ActionPolicy, Transition
    store.go            <- outbound.Store{db}, NewStore(db)
    queue_test.go, transition_test.go
  
  crawl/                <- Phase 3 (self-contained)
    intents.go, posts.go, groups.go, group_quality.go, data_sources.go, private_files.go
    
  knowledge/            <- Phase 4 (massive but isolated; some files may stay if shared)
    assets.go, sources.go, embeddings.go, cost.go, events.go, feedback.go, replay.go, soak.go, vector_query.go
    
  coordination/         <- Phase 5 (action_ledger + engagement + behaviour as one bounded context)
    action_ledger.go, engagement_reconcile.go, behaviour_profile.go, execution_attempts.go
    
  identities/           <- Phase 6 (accounts + sessions + tokens + facebook_status)
    accounts.go, sessions.go, session_status.go, agent_tokens.go, identities.go, facebook_status.go
    
  connectors/           <- Phase 7
    commands.go, ownership.go, pairing.go, streams.go
    selector_cache.go
    
  workspace/            <- Phase 8 (leads + threads + niches — hardest because of cross-domain SQL)
    leads.go, lead_engagement.go, threads.go, context_niches.go, classification_log.go
    
  prompts/              <- Phase 9 (prompts + routing + skills)
    memory.go, routing.go, skills.go
    
  users/                <- Phase 10 (foundational — last because everything depends on it)
    users.go, organization.go
    
  app/                  <- Phase 11 (misc: career_jobs, kpi, learning, media_assets, price_items, app_store, dedup)
```

**Top-level after extraction**: ~6-8 files (store.go, schema.go, migrator.go, backup.go, stats.go + Misc). Mỗi subpackage 4-15 files, 500-3500 LOC.

---

## 5. Phased rollout — 1 phase = 1 PR

### Phase 0 (DOC ONLY, 0 code change)
Tạo `internal/store/DOMAINS.md` ghi rõ mỗi file thuộc domain nào. Convention: file mới phải claim domain ở header comment. **Bắt buộc trước mọi phase code-change** sau.

**Cost**: ~1 hour. **Risk**: 0.

### Phase 1 — dbutil extraction (LOW RISK PROOF OF CONCEPT)
Extract helpers thuần (parseSQLiteTime, retryOnBusy, utcDayKey, boolToInt) + dialect_* files vào `internal/store/dbutil/`.

- 61 helper callsites → đổi import + tên (`parseSQLiteTime` → `dbutil.ParseSQLiteTime`)
- Helpers KHÔNG có state, KHÔNG dùng `*Store` → an toàn extract
- Validates the migration tooling and Go module structure

**Cost**: ~100 LOC change in 17 files (mechanical). **Risk**: low. **Wins**: proof the pattern works.

### Phase 2 — outbound/ extraction (proof of business-domain extraction)
Move 12 outbound files + tests vào `internal/store/outbound/`. Use **bridge pattern** to avoid 14 caller-site migrations across BE/extension.

- New: `outbound.Store{db}` with methods `Queue, Claim, Finalize, Reset, GetForOrg, ListForOrg, ...`
- Top-level `store.Store` keeps `QueueOutboundForOrg`, `ClaimPlannedOutboundForOrg`, etc. as thin delegates
- New code can use `s.outbound.Queue(...)` directly; old code keeps working
- Test files move with their code

**Cost**: ~2,200 LOC moved + ~150 LOC delegate wrappers. **Risk**: medium (changes import graph). **Wins**: outbound is fully encapsulated, can evolve independently.

### Phase 3 — crawl/ extraction (cleanest second extraction)
Crawl pipeline is self-contained — `crawl_intents` + posts/groups have minimal cross-domain calls.

**Cost**: ~1,300 LOC moved. **Risk**: low. **Wins**: validates pattern with another domain.

### Phase 4 — knowledge/ consolidation
Knowledge OS files (~3,553 LOC) already partially live in `internal/workspace_knowledge/`. This phase MERGES the store-side knowledge files into a `internal/store/knowledge/` subpackage that the workspace_knowledge layer imports.

**Cost**: ~3,500 LOC moved + restructure import. **Risk**: medium (knowledge_assets uses *Store methods extensively, may need composition adjustment). **Wins**: biggest single domain isolated.

### Phase 5 — coordination/ extraction
Action_ledger + behaviour_profile + execution_attempts + engagement_reconcile form a bounded context: "what did each account do, when, and was it verified?". Outbound depends on this — that's OK, outbound imports coordination.

**Cost**: ~2,000 LOC moved + ~10 callsite updates from outbound. **Risk**: medium (dependency order: coordination must be self-contained THEN outbound depends on it). **Wins**: clean execution-verification ownership.

### Phase 6+ — Defer until needed
identities, connectors, workspace (leads+threads), prompts, users, app — extract on demand when those domains get hot or when new features land. **Don't pre-extract**; the cost is real and benefit unclear until proven.

---

## 6. New v2.5 features should be born as subpackages

Per [TEMPLATE_TELEGRAM_COPILOT_PLAN.md](specs/TEMPLATE_TELEGRAM_COPILOT_PLAN.md), upcoming work:
- Template Library (PR-3 + PR-4)
- Telegram multi-tenant (PR-5)
- Copilot bridge (PR-6)
- Image attach (PR-7)
- Account Reputation projection (PR-2.5)

**Mandate**: ALL these should be NEW subpackages from day 1, not added to existing `internal/store/`.

```
internal/store/
  templates/       <- saved_templates table + broadcast_campaigns + broadcast_targets
  telegram/        <- org_telegram_bots + telegram_link_tokens
  copilot/         <- agent_sessions + agent_messages + agent_session_channels
  reputation/      <- account_reputation_snapshots
```

This sets the precedent: future domains are self-contained from day 1.

---

## 7. Cross-domain SQL strategy (the hard problem)

Hiện tại có 3 patterns cross-domain SQL:

### 7.1 EXISTS sub-projection
```sql
-- leads.go: EXISTS(SELECT 1 FROM outbound_messages WHERE ...) AS commented
```
**Resolution options**:
- **(A)** Leads package imports Outbound: `outboundFilter, err := outbound.HasCommentedForLeads(orgID, urls); ...` — Go-side N+1 join. Clean boundary, costs query count.
- **(B)** Raw SQL stays in leads.go but with table comment "tenant-ok: cross-domain projection". Pragmatic, less Go-pure.
- **(C)** Materialized projection on leads.commented_count column updated by outbound triggers. Heavy but fast.

**Đề xuất**: **(B) for now**, document the cross-domain SQL with a comment. Revisit if it becomes problematic.

### 7.2 Cross-package transaction
```go
// queueOutboundForOrgOnce calls recordActionLedgerTx in same tx
```
**Resolution**:
- Outbound package exports `Queue(tx *sql.Tx, ...)` variant
- OR: Coordination package exports `RecordActionLedger(tx *sql.Tx, ...)` and outbound imports
- Same tx flows through: caller (or store.Store facade) opens tx → passes to outbound.Queue → outbound passes to coordination.RecordActionLedger

**Đề xuất**: each subpackage method accepts an optional `*sql.Tx`. Top-level Store opens the tx and threads it through.

### 7.3 Shared helper functions
parseSQLiteTime, retryOnBusy — Phase 1 extraction to `dbutil/`. After that they're public + imported by every subpackage.

---

## 8. Test strategy

- Tests move WITH their package (Go convention: `foo.go` + `foo_test.go` in same package)
- Cross-package integration tests live at top-level `internal/store/` or in a new `integration_test/` folder
- `newSharedStore(t, ...)` helper used by many tests today — extract to `internal/store/testutil/`

---

## 9. Risks tổng quan

| Risk | Severity | Mitigation |
|------|----------|------------|
| Caller migration breaks production | High | Bridge pattern for Phase 2 (zero migration). Each phase is independent PR. |
| Cross-package tx complexity | Medium | Standardize: methods accept `*sql.Tx` as last arg; top-level Store opens tx |
| Cycles between subpackages | High | Strict dependency direction: foundational → upward. Phase 5 (coordination) ships before Phase 2 outbound depends on it cleanly |
| Helper duplication if dbutil not done first | Medium | Phase 1 MUST go first |
| Tests fail to compile after move | Medium | Run `go test ./...` after each phase, fix in same PR |
| Developer confusion during transition | Medium | Phase 0 DOMAINS.md doc + per-PR changelog |
| Schema.go (1123 LOC) stays in top-level | Low | Schema is bootstrap-only, never grows on hot path; leave alone |

---

## 10. Decisions

| Q | Choice | Rationale |
|---|--------|-----------|
| **Q1 Scope** | **C + E** — Phase 0+1+2 of existing store AND all v2.5 features born as subpackages | Best of both: validates pattern on existing code, mandates clean precedent for new code |
| **Q2 Outbound migration** | **A — Bridge pattern** | 14+ caller sites; bridge keeps callers stable. Wrappers are compat shims with expiry per L2. |
| **Q3 Naming inside subpackage** | **A — Short idiomatic names** (`outbound.Queue`, `outbound.Claim`) | Drop `ForOrg` suffix inside subpackages — tenant scoping is the only mode. **Tenant isolation (`org_id`) remains mandatory in every SQL query.** |
| **Q4 Cross-domain SQL** | **B — Document + defer** | `// tenant-ok: cross-domain projection` comments. Don't fix during extraction wave. |
| **Q5 Tests** | **A — Move with package** + extract `newSharedStore` to `internal/store/testutil/` | Go convention. |

## 10a. Delivery constraints (binding)

- `go test ./...` MUST pass after **every single phase** before next phase starts
- Phase 0 + Phase 1 ship as **one** PR (low-risk additive)
- Phase 2 ships as **separate** PR (verify Phase 0+1 first)
- Phase 3+ each is its own PR
- All 4 locked invariants (L1–L4 above) apply

---

## 11. Acceptance criteria (per phase)

Mỗi phase trước khi merge:
- `go build ./...` pass
- `go vet ./...` clean
- `go test ./...` all pass
- `bash scripts/check_tenant_isolation.sh` clean
- `npm --prefix frontend run build` pass
- No new package cycles (Go compiler catches)
- DOMAINS.md updated with new layout
- No file > 350 LOC after phase ends (target ~150)

---

## 12. Non-goals

- **KHÔNG** refactor leads/threads (Phase 8) trong wave này — cross-domain SQL coupling
- **KHÔNG** split schema.go — bootstrap-only, không hot path
- **KHÔNG** introduce interfaces for testability (separate concern; Go's structural typing + integration tests cover it)
- **KHÔNG** change DB layout — pure code organization
- **KHÔNG** rename DB tables — that's a separate migration
- **KHÔNG** extract to standalone Go modules — same repo, just subpackages
- **KHÔNG** refactor in any way that changes public HTTP / RPC API
- **KHÔNG** touch frontend or extension

---

## 13. Recommendation (TL;DR)

Trên `staged_evolution_over_big_bang` rule:

**Tôi đề xuất kết hợp 2 hướng**:

1. **Phase 0 + Phase 1 ngay** — DOMAINS.md + dbutil extraction. Total: ~1 PR, ~200 LOC change, low risk. **Validates pattern và improves discoverability immediately.**

2. **All NEW v2.5 features as subpackages từ day 1** — templates/, telegram/, copilot/, reputation/. Mandatory rule for new code.

3. **Phase 2 (outbound extraction) when calendar allows** — separate PR, bridge pattern, ~2-3 days of careful work.

4. **Phase 3+ defer until each domain proves needs** — không pre-extract.

**Không recommend** big-bang extraction of all 10 domains trong 1 wave. Vi phạm staged_evolution_over_big_bang rule.
