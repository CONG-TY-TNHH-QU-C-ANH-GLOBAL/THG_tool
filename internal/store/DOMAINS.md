# `internal/store/` — Domain Ownership Index

**Phase 0 of [STORE_SUBPACKAGE_REFACTOR](../../specs/STORE_SUBPACKAGE_REFACTOR.md)**
**Status**: docs-only ownership map. No code moves in Phase 0. File headers are tagged with their domain (see `// Domain:` comment at the top of each `.go` file).
**Last full audit**: 2026-05-21 — 89 files, 22,949 LOC, 13 domains.

## How to use this index

- **Adding a new file to `internal/store/`**: pick a domain from §2 and put `// Domain: <name>` as the first non-package-doc comment. If no domain fits, propose a new bucket in PR review BEFORE writing code.
- **Adding a new domain**: not in `internal/store/` directly. Per the [V2.5 precedent](../../specs/STORE_SUBPACKAGE_REFACTOR.md#6-new-v25-features-should-be-born-as-subpackages), new domains (templates, telegram, copilot, reputation, ...) must be born as `internal/store/<domain>/` subpackages from day 1.
- **Renaming / moving a file across domains**: update both the file's `// Domain:` header AND this index in the same PR.

## Reading order if you're new

1. **infra** (5 files) — Store struct, schema bootstrap, migration framework. Start here to understand the DB layout.
2. **dbutil** (7 files) — pure helpers everyone calls (`parseSQLiteTime`, `retryOnBusy`, dialects). No business logic.
3. **users** (2 files) — tenant root (`users`, `organizations`). Every other domain assumes orgID exists here.
4. **outbound** (9 files) — execution machinery, just refactored (PR-1 + PR-2). Reference shape for future subpackages.
5. **coordination** (8 files) — `action_ledger`, `execution_attempts`, behaviour profile, engagement reconcile.
6. Other domains as needed.

---

## 1. Dependency direction (locked by [feedback-store-subpackage-locks](../../../../C:/Users/ACER/.claude/projects/d--THG-THG-sale/memory/feedback_store_subpackage_locks.md) **L1**)

When extraction happens, packages depend on each other **foundational → upward only**. No cycles. Current direction:

```
            dbutil  ←  EVERYTHING
              ↑
           infra (Store, schema)
              ↑
           users (tenant root)
              ↑
        ┌─────┴─────┐
   coordination  identities  threads  prompts
        ↑           ↑          ↑        ↑
        └─────┬─────┴──────────┴────────┘
              │
        ┌─────┼──────────┐
     leads  outbound   crawl
        ↑               ↑
        └──── knowledge ┘   (knowledge mostly self-contained; few cross-imports)
        
                  app (misc — usually depends on most other domains)
                  
                  connectors (Chrome extension bridge — depends on identities + outbound)
```

**Rule**: a domain X may import a domain Y only if Y sits **strictly below** X in the diagram above. Same-level imports forbidden (would risk cycles). When in doubt: don't import.

---

## 2. The 13 domains

### **dbutil** — Pure DB helpers (7 files, 533 LOC)
Lowest layer. No business logic. No `*Store` methods. Everyone imports.

| File | LOC | What it does |
|------|-----|--------------|
| clear_db_test.go | 18 | Test util: clears DB tables for testing |
| dedup.go | 13 | Pure hash helper: compound dedup hash generation |
| dialect.go | 150 | DB dialect abstraction (SQLite/Postgres placeholder/time/interval differences) |
| dialect_postgres.go | 51 | Postgres dialect implementation |
| dialect_sqlite.go | 45 | SQLite dialect implementation |
| dialect_test.go | 188 | Tests dialect rebind + interval expressions on both drivers |
| postgres_driver.go | 26 | Pure constant: postgres driver name |
| sqlite.go | 75 | Pure helpers: parseSQLiteTime, isSQLiteBusy detection, retryOnBusy logic |

**Phase 1 target**: extract to `internal/store/dbutil/` (this PR).

---

### **infra** — Store struct + schema + migrator (5 files, 1,641 LOC)
Foundational. Owns the `*Store` struct, DB connection, schema bootstrap, migrations.

| File | LOC | What it does |
|------|-----|--------------|
| backup.go | 90 | Auto-backup scheduler: daily SQLite backups, 7-day retention |
| migrator.go | 270 | Schema migration runner: version registry, baseline detection |
| schema.go | 1184 | Legacy schema bootstrap: 150+ CREATE TABLE (idempotent) |
| schema_migrate_test.go | 88 | Tests migration runner state tracking |
| schema_template_test.go | 103 | Tests schema template compilation + marker version |
| store.go | 200 | Store struct, NewStore bootstrapper, dialect selection, Close |

**Stays in top-level `internal/store/`** even after full subpackage extraction. This is the composition root.

---

### **users** — Tenant root (2 files, 551 LOC)
Auth + organizations. Foundational because every other domain assumes `org_id` and `user_id` exist here.

| File | LOC | What it does |
|------|-----|--------------|
| organization.go | 129 | Org CRUD: create, get by ID/domain, plan tier, account limits |
| users.go | 422 | User CRUD + auth: email lookup, token hashing, login tracking |

---

### **outbound** — Execution machinery (9 files + 2 tests, 1,766 LOC)
Recently refactored (PR-1 + PR-2 of V2 outbound work). Reference shape for future subpackage extractions.

| File | LOC | What it does |
|------|-----|--------------|
| action_policy.go | 200 | Action type policies (dedup scope, cooldown, blocking rules) |
| outbound.go | 113 | Shell: `OutboundGuardDecision` + cross-cutting guards |
| outbound_claim.go | 148 | Claim path: planned→executing, execution_id token, lease |
| outbound_dedup.go | 296 | Dedup gate: per-account / workspace cooldown, daily limits |
| outbound_edit.go | 49 | Content edits + delete: planned-state mutations only |
| outbound_finalize.go | 145 | Terminal CAS: executing→finished/expired, verification outcome |
| outbound_lease.go | 128 | Lease eviction: reset stale executing rows |
| outbound_query.go | 212 | Read-side: get/list/count/scan helpers, org-scoped |
| outbound_queue.go | 198 | Write-side: InsertOutboundMessage, QueueOutboundForOrg |
| outbound_queue_test.go | 438 | Tests queue gates + dedup + autonomous-first |
| outbound_transition.go | 147 | Audit transitions: plan/claim/finalize/reset rows |
| outbound_transition_test.go | 350 | Tests transition recording + state machine |

**Phase 2 target**: extract to `internal/store/outbound/` (next PR).

---

### **coordination** — "What happened + was it verified" plane (4 files + 4 tests, 2,011 LOC)
Action_ledger + execution_attempts + behaviour_profile + engagement reconciliation. Sits below outbound (outbound depends on it for behaviour caps + ledger writes).

| File | LOC | What it does |
|------|-----|--------------|
| action_ledger.go | 233 | Coordination Plane ledger: records every action attempt |
| action_ledger_test.go | 194 | Tests action ledger writes + outcome tracking |
| behaviour_profile.go | 330 | Account behavior profiles: trust levels, caps, runtime counters |
| behaviour_profile_test.go | 304 | Tests behavior profile caps + day-rollover |
| engagement_reconcile.go | 183 | Repair false-positive "touched" states on action_ledger |
| engagement_reconcile_test.go | 191 | Tests engagement reconciliation |
| execution_attempts.go | 415 | Audit ledger for execution: transitions + DOM evidence |
| execution_attempts_test.go | 354 | Tests execution audit trail + transition state machine |

**Phase 5 target** (per design doc; not in this PR).

---

### **leads** — Lead pipeline (4 files + 2 tests, 1,509 LOC)
Lead CRUD + classification + engagement projection + niches. Has cross-domain SQL projection into outbound (per [STORE_SUBPACKAGE_REFACTOR §7.1](../../specs/STORE_SUBPACKAGE_REFACTOR.md#71-exists-sub-projection) — documented as `// tenant-ok: cross-domain projection`).

| File | LOC | What it does |
|------|-----|--------------|
| classification_log.go | 199 | AI classification audit trail: kept/rejected/error per crawl |
| context_niches.go | 114 | User context settings: key-value for lead niche configuration |
| lead_engagement.go | 380 | Lead engagement projection from action_ledger + execution_attempts |
| lead_engagement_test.go | 380 | Tests engagement state logic + visibility |
| leads.go | 418 | Lead CRUD: classify, query, repair source URLs |
| leads_repair_test.go | 132 | Tests lead source URL repair logic |

**Phase 8 target** (deferred — cross-domain coupling).

---

### **knowledge** — Knowledge OS internal store (10 files + 6 tests, 4,528 LOC)
Biggest domain. Layered architecture: sources → assets → embeddings → vector queries → events/feedback/replay/soak. Mostly self-contained; minimal cross-imports.

| File | LOC | What it does |
|------|-----|--------------|
| knowledge_assets.go | 363 | Layer 3: asset CRUD, org isolation, state filtering |
| knowledge_assets_test.go | 410 | Tests asset CRUD + state transitions |
| knowledge_cost.go | 201 | Cost accounting: embedding batch costs per source |
| knowledge_embeddings.go | 336 | Layer 2.5: embedding state machine, pending queue |
| knowledge_embeddings_test.go | 233 | Tests embedding state machinery |
| knowledge_events.go | 359 | Layer 7: event recording (sync, retrieval, outcome) |
| knowledge_events_test.go | 150 | Tests event recording + retrieval |
| knowledge_feedback.go | 195 | Goal G10: append-only human feedback |
| knowledge_replay.go | 304 | Operator Replay: read-side retrieval timeline |
| knowledge_replay_test.go | 171 | Tests replay queries + outcome tracking |
| knowledge_soak.go | 231 | Soak metrics: hit rate, fallback rate, scoring |
| knowledge_soak_test.go | 134 | Tests soak metrics + aggregation |
| knowledge_sources.go | 309 | Layer 1: source CRUD, re-sync idempotence |
| knowledge_sources_test.go | 254 | Tests source CRUD + state isolation |
| knowledge_vector_query.go | 185 | Layer 4: pgvector ANN queries (tenant-scoped) |
| data_sources.go | 111 | Legacy pre-Knowledge-OS connector registry. Reassigned from crawl 2026-05-21 — consumed by agent_brain / skills / autoflow handlers. Will be folded into knowledge_sources or deprecated in a future PR. |

**Phase 4 target**.

---

### **crawl** — Crawl pipeline (6 files + 1 test, 1,289 LOC)
Market intelligence crawl: intents, posts, groups, quality, data sources, private files.

| File | LOC | What it does |
|------|-----|--------------|
| crawl_intents.go | 549 | Recurring crawl plans: scheduling, status, execution |
| crawl_intents_test.go | 327 | Tests crawl intent lifecycle |
| group_quality.go | 162 | Group quality scoring: relevance, professionalism |
| groups.go | 105 | Crawl target groups: add, query, dedup by URL |
| posts.go | 101 | Crawl posts: insert with dedup, query recent |
| private_files.go | 45 | Private knowledge files for crawl (org-scoped) |

**Note 2026-05-21**: `data_sources.go` was reassigned from crawl → **knowledge** domain during Phase 3 audit. The `data_sources` table is a legacy pre-Knowledge-OS connector registry consumed by agent_brain / skills / autoflow handlers, not by the crawl scheduler. Co-locating with the knowledge domain matches the architectural intent even though the table predates the Knowledge OS layered design.

**Phase 3 target** (cleanest second extraction).

---

### **identities** — FB account lifecycle (5 files + 2 tests, 1,088 LOC)
FB accounts, sessions, agent tokens, browser identity. Foundational for outbound (behaviour caps require account state).

| File | LOC | What it does |
|------|-----|--------------|
| accounts.go | 345 | FB account lifecycle: add, get, query + encryption |
| accounts_identity_test.go | 71 | Tests identity account lookups + multi-org isolation |
| accounts_rbac_test.go | 75 | Tests account RBAC + assignment |
| agent_tokens.go | 326 | Browser agent tokens: identity, capabilities |
| facebook_status.go | 25 | FB status summary: connected, group count, daily leads |
| session_status.go | 84 | Browser session status enum |
| sessions.go | 130 | Chrome extension browser session lifecycle |

**Phase 6 target**.

---

### **connectors** — Chrome extension bridge (5 files, 653 LOC)
Extension command bus + pairing + ownership + selector cache.

| File | LOC | What it does |
|------|-----|--------------|
| connector_commands.go | 199 | Chrome extension command queue |
| connector_ownership.go | 48 | Validates agent ownership of account stream |
| connector_pairing.go | 172 | Extension pairing code generation + lifecycle |
| connector_streams.go | 143 | Extension screenshot persistence + state tracking |
| selector_cache.go | 91 | CSS/XPath selector cache: LLM-discovered selectors |

**Phase 7 target**.

---

### **threads** — Inbox / conversations (1 file, 291 LOC)

| File | LOC | What it does |
|------|-----|--------------|
| threads.go | 291 | Conversation threads: CRUD + last_outbound tracking |

**Phase 8 target** (bundles with leads).

---

### **prompts** — AI prompt machinery (3 files + 1 test, 826 LOC)
Prompt memory, routing analysis, skills.

| File | LOC | What it does |
|------|-----|--------------|
| prompt_memory.go | 181 | Prompt audit: scan logs, inbox messages, routing decisions |
| prompt_routing.go | 419 | Orchestrator observability: conflict candidates, ask-back |
| prompt_routing_test.go | 233 | Tests routing heuristic detection |
| skills.go | 224 | Skill execution machinery: org_skills, skill_executions |

**Phase 9 target**.

---

### **app** — Misc application concerns (8 files, 1,096 LOC)
Tasks, KPI, learning, media, pricing, careers, stats, browser fingerprints.

| File | LOC | What it does |
|------|-----|--------------|
| app_store.go | 410 | Task execution + task-derived leads (AppTask, TaskLead) |
| career_jobs.go | 78 | HR playbook legacy: job postings |
| identities.go | 104 | Browser identity fingerprints (AppStore): UA, screen, WebGL |
| kpi.go | 107 | Staff KPI tracking: conversations, conversions, points |
| learning.go | 152 | Adaptive weights + feedback outcomes |
| media_assets.go | 123 | Company image asset library |
| price_items.go | 75 | Pricing intelligence: service name, price, unit |
| stats.go | 36 | Dashboard stats aggregation (legacy) |

**Phase 11 target** (last — heterogeneous bucket).

---

## 3. New domains (V2.5 precedent — locked)

Per [feedback-store-subpackage-locks](../../../../C:/Users/ACER/.claude/projects/d--THG-THG-sale/memory/feedback_store_subpackage_locks.md) **mandate**: every new domain ships as `internal/store/<domain>/` from day 1. NO new files in top-level `internal/store/`.

Planned (per [TEMPLATE_TELEGRAM_COPILOT_PLAN](../../specs/TEMPLATE_TELEGRAM_COPILOT_PLAN.md)):

- `internal/store/templates/` — `saved_templates`, `broadcast_campaigns`, `broadcast_targets`
- `internal/store/telegram/` — `org_telegram_bots`, `telegram_link_tokens`
- `internal/store/copilot/` — `agent_sessions`, `agent_messages`, `agent_session_channels`
- `internal/store/reputation/` — `account_reputation_snapshots`

---

## 4. File counts (verification)

| Domain | Files | LOC |
|--------|------:|----:|
| dbutil | 7 | 533 |
| infra | 5 | 1,641 |
| users | 2 | 551 |
| outbound | 12 | 2,066 |
| coordination | 8 | 2,011 |
| leads | 6 | 1,509 |
| knowledge | 15 | 4,528 |
| crawl | 7 | 1,289 |
| identities | 7 | 1,088 |
| connectors | 5 | 653 |
| threads | 1 | 291 |
| prompts | 4 | 826 |
| app | 8 | 1,096 |
| **TOTAL** | **89** | **22,949** ✓ |
