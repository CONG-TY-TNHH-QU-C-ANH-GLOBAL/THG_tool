# `internal/store/` — Domain Ownership Index

**Status**: living doc — describes code-as-of-now, not history. Migration phases, audit findings, and roadmap live in [specs/STORE_SUBPACKAGE_REFACTOR.md](../../specs/STORE_SUBPACKAGE_REFACTOR.md). Per-phase rationale stays in that doc + memory; this file is the day-to-day navigation map.

**Last verified**: 2026-05-21 (Phase 4 knowledge extraction complete).

## How to use this index

- **Adding a new file**: extracted subpackages → put in `internal/store/<domain>/`. Top-level domains → keep in top-level + add `// Domain: <name>` header. New domains (templates, telegram, copilot, ...) MUST ship as subpackages from day 1.
- **Importing from a domain**: cross-domain access goes through the parent `Store` accessor: `s.Outbound().Foo()`, `s.Knowledge().Bar()`, etc. No reaching into peers' internals.
- **Adding tests**: subpackage tests live in `package <domain>_test` and consume `storetest.CopyTemplate` (see §3.9). Top-level tests use `newSharedStore` (same trick under the hood).

---

## 1. Quick navigation

| Domain | Location | Accessor | Status |
|--------|----------|----------|--------|
| **infra** | `internal/store/` | `store.New(...)` | always top-level (composition root) |
| **dbutil** | `internal/store/dbutil/` | `dbutil.NewSQLiteDialect()` | extracted (Phase 1, 2026-05-21) |
| **storetest** | `internal/store/storetest/` | `storetest.CopyTemplate(...)` | extracted 2026-05-21 (test infra) |
| **outbound** | `internal/store/outbound/` | `s.Outbound()` | extracted (Phase 2, 2026-05-21) — bridge wrappers in `outbound_aliases.go` (L2 expiry: no new wrappers) |
| **crawl** | `internal/store/crawl/` | `s.Crawl()` | extracted (Phase 3, 2026-05-21) — clean-cut, no wrappers |
| **knowledge** | `internal/store/knowledge/` | `s.Knowledge()` | extracted (Phase 4, 2026-05-21) — clean-cut, no wrappers, tests in subpackage |
| **coordination** | `internal/store/coordination/` | `s.Coordination()` | extracted (Phase 5B, 2026-05-21) — clean-cut, no wrappers. Cross-package writes from outbound flow via Hooks closure pattern. |
| **prompts** | `internal/store/prompts/` | `s.Prompts()` | extracted (Phase 9, 2026-05-22) — clean-cut, no wrappers. Schema bootstrap exposes `prompts.Migrate(db *sql.DB)` for the pre-subpackage-construction call from `Store.migrate`. |
| **connectors** | `internal/store/connectors/` | `s.Connectors()` | extracted (Phase 7, 2026-05-22) — clean-cut, no wrappers. agent_tokens.go reclassified from identities domain (every type + method on it was already connector-domain). Schema bootstrap exposes `connectors.InitSelectorCache(db *sql.DB)` for the pre-construction selector_cache table create. |
| **identities** | `internal/store/identities/` | `s.Identities()` | extracted (Phase 6, 2026-05-22) — clean-cut, no wrappers. Scope-limited to accounts.go + facebook_status.go (the *Store-receiver files). identities.Store holds its own encKey (mirrored from parent via SetEncryptionKey) so cookie encryption stays in-domain. |
| **app** | `internal/store/app/` | `s.App()` | extracted (Phase 11 narrow, 2026-05-22) — only the *Store-receiver files (career_jobs, kpi, media_assets, price_items, stats). learning.go + app_store.go + sessions/identities.go (browser fingerprints) stay at top-level because they use the legacy *AppStore wrapper. |
| **threads** | `internal/store/threads/` | `s.Threads()` | extracted (Phase 8a, 2026-05-22) — single-file clean-cut. conversation_threads + conversation_messages. The conversationGateForOutbound adapter stays at parent store level (top-level) because it composes threads + outbound. |
| **leads** | `internal/store/leads/` | `s.Leads()` | extracted (Phase 8b, 2026-05-22) — clean-cut. 4 files (leads, lead_engagement, classification_log, context_niches). leads.Store holds a *threads.Store handle for the engagement-projection cross-domain reads (per DOMAINS.md §2.2 cross-domain projections via tenant-ok annotations). |
| users | `internal/store/` | direct methods | top-level (foundational, may stay) |
| leads | `internal/store/` | direct methods | top-level (Phase 8 — cross-domain SQL coupling) |
| identities | `internal/store/` | direct methods | top-level (Phase 6) |
| connectors | `internal/store/` | direct methods | top-level (Phase 7) |
| threads | `internal/store/` | direct methods | top-level (Phase 8) |
| prompts | `internal/store/` | direct methods | top-level (Phase 9) |
| app | `internal/store/` | direct methods | top-level (Phase 11 — heterogeneous bucket) |

---

## 2. Dependency direction (locked invariant L1)

```
                       dbutil  ←  EVERYTHING
                          ↑
                    infra (Store, schema, migrations)
                          ↑
                       users (tenant root)
                          ↑
       ┌────────────┬─────┴──────┬──────────┬──────────┐
   coordination  identities    threads    prompts     app
       ↑              ↑           ↑          ↑
       └──────┬───────┘
              │
       ┌──────┼──────┐
     leads  outbound  crawl
       ↑                ↑
       └─── knowledge ──┘

   connectors (Chrome extension bridge) ← depends on identities + outbound
   storetest ← store (test-only; bootstrap-injection prevents prod cycle)
```

### 2.1 Strict-below rule

Domain X may import domain Y **only if Y sits strictly below X** in the diagram. Same-level imports are forbidden (cycle risk). Go's compiler catches cycles, but the design must not require them.

### 2.2 No bidirectional domain knowledge (locked 2026-05-21)

Per [[feedback_no_bidirectional_domain_knowledge]]: downstream domains consume upstream via **projections / contracts** ONLY. Upstream MUST NOT import downstream runtime semantics.

| Allowed | Forbidden |
|---------|-----------|
| outbound imports coordination's `ActionLedger` type to write entries | coordination imports outbound's `OutboundMessage` type |
| leads' engagement projection joins outbound rows (documented `// tenant-ok` SQL) | outbound's runtime queries leads.engagement_state |
| knowledge writes its own events table | knowledge reads outbound/coordination internals |

Cross-domain reads in SQL (`leads JOIN outbound_messages`) are accepted as projections — **flagged with `// tenant-ok: cross-domain projection (X -> Y)`** so reviewers can audit blast radius. Cross-domain *writes* require explicit Hooks struct + design-doc justification.

### 2.3 storetest is one-way

`storetest` imports `store` for its `Bootstrap` injection consumers — but it is a TEST-ONLY package (regular package, not `_test.go`, but never linked into production binaries). Production code MUST NOT import storetest. See [[feedback_storetest_scaling_pattern]].

### 2.4 Truth ownership matrix (locked 2026-05-21)

Each runtime "truth" has exactly **one** canonical owner. Every other place that mentions the truth is a projection (read-only view, derived in SQL or by re-computation). Writes to a truth go through the owner's API only.

| Truth | Canonical Owner (table → domain) | Allowed Writers | Projection Consumers (read-only) |
|-------|----------------------------------|-----------------|----------------------------------|
| action history | `action_ledger` → coordination | coordination methods only (callable from outbound queue/finalize, reconcile) | leads engagement projection (`// tenant-ok` SQL), badges, reputation snapshots |
| execution verification | `execution_attempts` → coordination | coordination methods only (called from outbound finalize path) | engagement projection, ledger replay UI |
| behavioural runtime counters | `account_behaviour_runtime` → coordination | coordination's `IncrementRuntimeCounter` (called from outbound queue) | outbound dedup gate (cap check) |
| engagement correction | revocation events into `action_ledger` → coordination/reconcile | reconcile only (emits engagement_revoked events; never UPDATE/DELETE existing rows) | leads engagement projection re-derivation |
| outbound execution state | `outbound_messages.execution_state` → outbound | outbound state machine (queue, claim, finalize, lease) | NA — only outbound queries it. The cross-domain projection is `verification_outcome`, derived from execution_attempts. |
| lead engagement projection | `leads.engagement_state` (derived) → leads | leads projection logic (SQL derives from action_ledger + execution_attempts) | dashboards, UI badges |
| knowledge retrieval events | `knowledge_events` (retrieval rows) → knowledge | knowledge.RecordRetrievalWithTrace | replay UI, soak metrics |
| embedding state | `knowledge_assets.embedding_*` columns → knowledge | knowledge.embedding worker | retrieval rerank, soak drift detection |

**Rules implied by the matrix:**

1. Coordination owns runtime truth. Outbound and leads are PROJECTIONS over coordination's tables. Per §2.2, coordination does NOT read outbound/leads back.
2. Reconcile is the ONLY author of revocation events. Reconcile lives INSIDE coordination — not a separate domain — so the action_ledger append-only invariant remains single-source.
3. Every cross-domain SQL JOIN crossing these ownership lines requires a `// tenant-ok: cross-domain projection (<owner> -> <consumer>)` annotation. Reviewer audit gate.
4. New truths must be added to this matrix BEFORE the schema migration that introduces them lands. Adding a truth post-hoc invites silent ownership drift.

If a cell becomes contested (two writers, or owner unclear), the runtime is broken. Resolve ownership in a design-doc PR before merging any code that exercises the contested truth.

---

## 3. Subpackage contract (binding for every extraction)

Every `internal/store/<domain>/` package MUST satisfy all nine:

1. **Single entry point** — exports `Store` struct + `NewStore(db *sql.DB, dialect dbutil.Dialect) *Store`. No package-level globals.
2. **Tenant isolation** — every public SQL query filters by `org_id = ?` (verified by `scripts/check_tenant_isolation.sh`). No exceptions; even superadmin queries take orgID.
3. **Dependency direction** — imports `internal/store/dbutil` + lower domains only. Never the parent `store` package. Compile-time enforcement.
4. **Tx threading** — opens its own tx for intra-domain cascades. Accepts external `*sql.Tx` ONLY when cross-package writers exist and the threading is audited (locked L3). Never silently opens nested transactions.
5. **No abstraction theater** — concrete `*sql.DB` + dialect. No repository interfaces, no DI container, no plugin/event-bus, no service locator (locked L4).
6. **Interface threshold** — define an interface only when ≥2 production implementations exist. Test-only fakes go in `package <pkg>_test`, not the production package.
7. **Public API only** — handlers reach the subpackage via the `Store.<Xxx>()` accessor. Subpackage methods that need to be reachable MUST be exported; nothing leaks through unexported types.
8. **Cross-domain writes documented** — every write that touches another domain's table requires a `// tenant-ok: cross-domain projection (<src> -> <dst>)` comment + design-doc entry. Reviewer's checklist.
9. **External tests via storetest** — tests live in `internal/store/<domain>/` as `package <domain>_test`. Schema bootstrap via `storetest.CopyTemplate` (5-line binding per binary, never duplicate the template trick).

Reference implementation: **knowledge/** (Phase 4). Every future extraction should match its shape.

---

## 4. Domain definitions

### **infra** — Composition root (top-level, always)

`store.New(...)`, schema bootstrap, migrations, backup, encryption key. Owns the `*Store` struct. Subpackages compose under it via accessor methods.

Key files: `store.go`, `schema.go`, `migrator.go`, `backup.go`, `dialect.go` (top-level shim), `helpers.go`.

### **dbutil** — Pure DB helpers (`internal/store/dbutil/`)

Cross-cutting utilities every package needs: `ParseSQLiteTime`, `BoolToInt`, dialect abstraction (SQLite vs Postgres). Lowest layer; everything imports.

### **storetest** — Shared test infrastructure (`internal/store/storetest/`)

Schema-template trick (sync.Once compile + per-test copy). Consumed by every subpackage's integration tests via bootstrap-injection. Single source of truth — never duplicate per subpackage. See [[feedback_storetest_scaling_pattern]].

### **outbound** — Execution machinery (`internal/store/outbound/`)

V2 outbound state taxonomy (execution_state ⊥ verification_outcome). Action policies, queue/claim/finalize/lease/dedup state machine, transition audit. Tests in `internal/store/outbound_*_test.go` (top-level, internal access) — Phase 2 fallback.

Bridge wrappers in `outbound_aliases.go` exist as L2 transition shims. **No new wrappers per L2 lock.** New code calls `s.Outbound().Foo()` directly.

### **crawl** — Crawl pipeline (`internal/store/crawl/`)

CrawlIntent scheduler (recurring crawl plans), groups + group_quality, posts (with dedup), private_files. Clean-cut extraction; no top-level wrappers. Test stays in top-level (`crawl_intents_test.go`) per Phase 3 fallback (uses unexported `getCrawlIntentByHash`).

### **knowledge** — Workspace Knowledge OS (`internal/store/knowledge/`)

Layered architecture: sources (L1) → assets (L3) → embeddings (L2.5) → vector queries (L4) → events / feedback / replay / soak / cost (L7). Methods dropped the redundant `Knowledge` prefix on extraction (`GetKnowledgeAsset` → `GetAsset`). Zero cross-domain writes by audit; no Hooks struct needed.

Tests in subpackage as `package knowledge_test` (external) + `pgvector_literal_test.go` as `package knowledge` (internal — tests unexported `pgVectorLiteral`).

### **users** — Tenant root (top-level)

Auth + organizations. Foundational; every other domain assumes `org_id` and `user_id` resolve here.

Files: `organization.go`, `users.go`.

### **coordination** — "What happened + was it verified" plane (`internal/store/coordination/`)

action_ledger + execution_attempts + behaviour_profile + engagement_reconcile + behaviour_caps + execution_transition_writer. The runtime-truth substrate.

**Extracted Phase 5B 2026-05-21**. Clean-cut, no bridge wrappers. Cross-package writes from outbound flow through the Hooks closure pattern wired in `installOutboundHooks` (`internal/store/outbound_aliases.go`). Per L1 + [[feedback_no_bidirectional_domain_knowledge]] coordination imports no peer domain — outbound types are unpacked to primitives at the wiring point.

Pre-existing append-only violations (MarkActionLedgerOutcome* + engagement_reconcile UPDATEs) carried as documented debt; the append-only enforcement fix is a follow-up PR per [[feedback_append_only_correction_events]].

Files: `action_ledger.go`, `behaviour_caps_check.go`, `behaviour_profile.go`, `engagement_reconcile.go`, `execution_attempts.go`, `execution_transition_writer.go`, `store.go`. Tests: `action_ledger_internal_test.go` (internal — unexported helper), `engagement_reconcile_test.go`, `execution_attempts_test.go` (external `package coordination_test`), `testing_helpers_test.go` (storetest binding). Cross-domain tests (`action_ledger_test.go`, `behaviour_profile_test.go`) stay at top-level alongside outbound.

### **leads** — Lead pipeline (top-level)

Lead CRUD + classification + engagement projection + niches. Has cross-domain SQL projection into outbound (`EXISTS(SELECT FROM outbound_messages ...)`) — documented `// tenant-ok: cross-domain projection (outbound -> leads)`. See [STORE_SUBPACKAGE_REFACTOR §7.1](../../specs/STORE_SUBPACKAGE_REFACTOR.md).

Files: `classification_log.go`, `context_niches.go`, `lead_engagement.go`, `leads.go` (+ tests).

### **identities** — FB account lifecycle (top-level)

FB accounts, sessions, agent tokens, browser identity. Foundational for outbound (behaviour caps require account state).

Files: `accounts.go`, `agent_tokens.go`, `facebook_status.go`, `session_status.go`, `sessions.go` (+ tests).

### **connectors** — Chrome extension bridge (top-level)

Extension command bus + pairing + ownership + selector cache.

Files: `connector_commands.go`, `connector_ownership.go`, `connector_pairing.go`, `connector_streams.go`, `selector_cache.go`.

### **threads** — Inbox / conversations (top-level)

Conversation threads: CRUD + last_outbound tracking. Single file — extraction bundled with leads.

Files: `threads.go`.

### **prompts** — AI prompt machinery (top-level)

Prompt memory, routing analysis, skill executions.

Files: `prompt_memory.go`, `prompt_routing.go`, `skills.go` (+ tests).

### **app** — Misc application concerns (top-level, heterogeneous)

Tasks, KPI, learning, media, pricing, careers, stats, browser fingerprints. Last extraction candidate; will likely split rather than extract whole.

Files: `app_store.go`, `career_jobs.go`, `identities.go` (NOTE: different from `identities/`-domain `accounts.go` — browser fingerprints lift here for now), `kpi.go`, `learning.go`, `media_assets.go`, `price_items.go`, `stats.go`.

### Legacy / cross-domain residue

- `data_sources.go` — pre-Knowledge-OS connector registry. Reassigned to knowledge domain in Phase 3 audit; physically stays in top-level until folded into knowledge or deprecated.

---

## 5. New domains policy (V2.5 precedent — locked)

Per [[feedback_store_subpackage_locks]] **mandate**: every new domain ships as `internal/store/<domain>/` from day 1. **NO new files in top-level `internal/store/`** for greenfield domains.

Planned (per [TEMPLATE_TELEGRAM_COPILOT_PLAN](../../specs/TEMPLATE_TELEGRAM_COPILOT_PLAN.md)):

- `internal/store/templates/` — saved_templates, broadcast_campaigns, broadcast_targets
- `internal/store/telegram/` — org_telegram_bots, telegram_link_tokens
- `internal/store/copilot/` — agent_sessions, agent_messages, agent_session_channels
- `internal/store/reputation/` — account_reputation_snapshots

Each MUST conform to §3 subpackage contract.

---

## 6. File counts (verification, 2026-05-21)

Extracted subpackages:

| Subpackage | Production files | Test files | Production LOC |
|------------|-----------------:|-----------:|---------------:|
| dbutil | 4 | 1 | 351 |
| storetest | 1 | 0 | 165 |
| outbound | 11 | 0 (tests at top-level) | 1,478 |
| crawl | 6 | 0 (test at top-level) | 1,066 |
| knowledge | 10 | 8 | 2,551 |

Top-level remaining: **56 `.go` files** (production + tests) across infra, users, coordination, leads, identities, connectors, threads, prompts, app, plus subpackage-test-fallback files (outbound_*_test.go, crawl_intents_test.go).

For per-phase migration history + audit findings see [specs/STORE_SUBPACKAGE_REFACTOR.md](../../specs/STORE_SUBPACKAGE_REFACTOR.md).
