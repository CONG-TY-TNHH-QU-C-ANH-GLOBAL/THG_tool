---
doc_type: architecture
status: active
owner: platform
last_reviewed: 2026-06-28
related_pr_or_issue: chore/docs2-architecture-backlinks-frontmatter
---

# Architecture Diagram Reconciliation

> Part of the [architecture docs index](INDEX.md).

## Status

```text
Status: Active architecture companion document.
Source diagram: .claude/detailed_architecture_v2.png.
Scope: north-star reconciliation, not direct implementation spec.
Binding priority: docs/architecture/* overrides the diagram on conflict.
```

**Created:** 2026-06-23. **Track:** Architecture governance (docs-only).

This document reconciles the high-level architecture image
[`.claude/detailed_architecture_v2.png`](../../.claude/detailed_architecture_v2.png)
with the binding architecture contract under `docs/architecture/`. The diagram
was visually inspected at native resolution (13436×4852, read in zoomed tiles);
every component named in §1 was read from the rendered labels, not inferred from
the filename.

The five governing rules of this document:

1. The diagram is a **north-star capability/topology map**.
2. The diagram is **not a direct implementation spec**.
3. The binding implementation contract is **`docs/architecture/*`**.
4. When the diagram conflicts with the architecture standard, **the standard wins**.
5. **No future agent may blindly refactor code based only on the diagram.**

**Binding docs this document defers to (authoritative on conflict):**

- [`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md) — the seven decisions, layer model, module catalogue.
- [`MODULE_BOUNDARIES.md`](./MODULE_BOUNDARIES.md) — per-module allowed/forbidden imports.
- [`PORTS_AND_ADAPTERS.md`](./PORTS_AND_ADAPTERS.md) — consumer-owned ports, composition-root wiring.
- [`CONNECTOR_STATE_MACHINE.md`](./CONNECTOR_STATE_MACHINE.md) — connector states + **pull-based** action lifecycle.
- [`DATABASE_OWNERSHIP.md`](./DATABASE_OWNERSHIP.md) — table → owner / readers / writers.
- [`CURRENT_CODE_AUDIT.md`](./CURRENT_CODE_AUDIT.md) — honest gap audit of code-as-of-now.
- [`CURRENT_PACKAGE_INVENTORY.md`](./CURRENT_PACKAGE_INVENTORY.md) — per-package status/target/phase.
- [`REFACTOR_ROADMAP.md`](./REFACTOR_ROADMAP.md) — staged path to the standard (Phases A–I).
- [`TRANSACTIONAL_OUTBOX.md`](./TRANSACTIONAL_OUTBOX.md) — durable outbox + relay + process managers.
- [`ADR-PR9-DATA-PLATFORM.md`](./ADR-PR9-DATA-PLATFORM.md) — SQLite→Postgres data-platform strategy.
- [`SONAR_FACTORY_PROTOCOL.md`](./SONAR_FACTORY_PROTOCOL.md) — Sonar cleanup risk lanes/policy (kept separate from architecture work).

---

## Executive Summary

The diagram is a useful **12–18 month capability direction**: a multi-tenant
Postgres/Redis data tier, a vector store for KnowledgeOS, object storage for
media, multi-platform automation behind platform adapters, and a separable AI
orchestration layer. It is worth keeping as a shared picture of where the
platform is heading.

The current repository is **already governed by an official architecture
standard** ([`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md) and its
companions). The right path is therefore **not** a microservice rewrite to match
the diagram; it is **staged modular-monolith evolution** along the existing
[`REFACTOR_ROADMAP.md`](./REFACTOR_ROADMAP.md) (Phases A–D substantially shipped;
Phase E — the transactional outbox — is the keystone gap).

Crucially, the diagram **omits the safety spine**. Where the diagram is silent or
contradicts it, the following are **mandatory and override the diagram**:

- outbound coordination spine,
- approval gates (outbound defaults to approval-required),
- connector **pull / CAS / lease** claim model,
- `human_required` on login wall / checkpoint,
- append-only **action ledger / execution attempts** (single writer),
- the **transactional outbox** target for durable cross-module events,
- **tenant isolation** (`org_id` ownership checks on every feature).

---

## 1. What the Diagram Shows

The diagram is organized into colored zones (layers), each containing components.

**Layers:**

- **Client Layer** (top band)
- **API & Gateway Layer**
- **AI & Agentic Layer**
- **Core Services (Go)**
- **Automation Services** (nested inside Core Services)
- **Crawler Cluster**
- **Data & State Layer**
- **External Platforms**

**Components (read from the rendered labels):**

| Zone | Components |
|---|---|
| Client Layer | `Next.js Web App UI`; `Chrome Extension (Bridge)` sits on the boundary toward External Platforms |
| API & Gateway | `Go API Gateway`, `WebSocket Manager` |
| AI & Agentic | `AI Copilot (RAG Architecture)`, `LLM Orchestrator (LangGraph/LangChain)`, `Content Gen Agent` |
| Core Services (Go) | `Task & Workflow Engine (Temporal/BullMQ)`, `Sourcing Service (Taobao/1688)`, `Workspace & Team Service`, `CDP Controller` |
| Automation Services | `AutoInboxService`, `AutoCommentingService`, `AutoPostingService`, `AutoReelsVideoService`, `Platform Adapters (FB, IG, TikTok, Zalo, YT)` |
| Crawler Cluster | `Distributed Crawler Nodes (Playwright/Docker)`, `Proxy Rotation Manager`, `Anti-Bot Bypass (Stealth/Captcha)`, `Supplier Analyzer (Python)` |
| Data & State | `Redis (Cache/Session/Queue)`, `Vector Database (Milvus/Pinecone)`, `PostgreSQL (Multi-tenant DB)`, `Object Storage (S3)` |
| External Platforms | `Social (FB/IG/TikTok/Zalo/YT)`, `Ecommerce (Taobao/1688)` |

**Flow labels visible on edges:** `GraphQL/REST`, `Query`, `Tool Call`,
`Retrieve Context`, `Vectorize Data`, `Task Dispatch`, `Scrape Request`,
`Control Commands`, `Raw Data`, `Browser Commands`, `WSS (Real-time)`,
`Real Browser Actions`, `Session Data`, `Scrape Data`,
`Platform-specific CDP Commands`, `Store Messages`, `Store Comments`,
`Store Posts`, `Store Reels/Videos`, `Media Upload`, `Store Images/Logs`,
`Store Supplier Data`, `User/WS Data`, `State/Queue`.

---

## 2. What the Current Repository Already Matches

Supported by [`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md),
[`CURRENT_CODE_AUDIT.md`](./CURRENT_CODE_AUDIT.md), and the package tree:

- **Go modular monolith with role split** — `cmd/scraper` (API), `cmd/worker`
  (crawler). A `cmd/agent` (connector/agent) role is *planned/aspirational only* —
  it has no tracked Go package today (no committed history). Matches "Core Services
  (Go)" + "Crawler Cluster" separation.
- **`org_id`-scoped multi-tenant direction** — tenant isolation is a binding rule
  and a green guard; matches the *property* behind "PostgreSQL (Multi-tenant DB)".
- **Platform service resolver/adapters direction** — `internal/platform/services/resolver/`
  has `facebook.go`, `taobao.go`, `alibaba1688.go` resolver stubs; matches
  "Platform Adapters" + "Sourcing Service (Taobao/1688)" *intent*.
- **AI layer split** — `internal/ai` (pure intelligence, models-only) vs
  `internal/drivers/copilot` (the Copilot/RAG driver, already extracted in the
  foundation sprint). Matches "AI Copilot" + "LLM Orchestrator" *conceptually*.
- **Chrome extension bridge** — `internal/browsergateway`, `internal/cdpclient`,
  `internal/server/agent`, and `local-connector-extension/`. Matches
  "Chrome Extension (Bridge)" + "CDP Controller".
- **Crawler / jobhandler structure** — `internal/jobs`, `internal/jobhandlers`,
  `internal/store/crawl`. Matches "Distributed Crawler Nodes".
- **Postgres/Redis direction as dev/scaffolded infrastructure (NOT runtime)** —
  dual-dialect layer (`internal/store/dialect.go`, `internal/store/postgres/`),
  [`ADR-PR9-DATA-PLATFORM.md`](./ADR-PR9-DATA-PLATFORM.md), and a dev compose at
  `deploy/dev/docker-compose.yml`. This is **scaffolding and strategy, not a
  production runtime switch**.

> Do **not** overclaim runtime support for PostgreSQL, Redis, vector DB, or S3.
> See §3.

---

## 3. What the Current Repository Does Not Match Yet

Per [`ADR-PR9-DATA-PLATFORM.md`](./ADR-PR9-DATA-PLATFORM.md) and
[`CURRENT_CODE_AUDIT.md`](./CURRENT_CODE_AUDIT.md):

- **Runtime database is SQLite.** `ADR-PR9` states plainly: "SQLite remains the
  current default (`DB_PATH=data/scraper.db`) … the only database the application
  runtime reads today." PostgreSQL is the *future* source of truth, reached via a
  later feature-flagged cutover — not wired into runtime now.
- **Redis is dev-only / ephemeral.** Present as dev infra; never an authoritative
  store for task/queue/ledger/proof/policy data.
- **Vector DB (Milvus/Pinecone) is not in the tree.** KnowledgeOS retrieval today
  does not depend on a separate Milvus/Pinecone service (see Open Questions on
  pgvector-vs-external-store).
- **Object Storage (S3) is not wired into runtime.** Media handling uses real
  uploaded files; there is no S3 adapter in the runtime path today.
- **Temporal / BullMQ is not implemented.** The repo uses a Go job/task pipeline +
  SQLite queue with in-memory composition-root callbacks; the durable substitute
  (transactional outbox + process managers) is **Phase E, not yet built** —
  `CURRENT_CODE_AUDIT.md` calls this "the biggest single gap".
- **LangGraph/LangChain sidecar is not implemented.** Orchestration is Go
  (`internal/agentloop`, `internal/drivers/copilot`). No Python LLM sidecar exists.
- **Multi-platform automation beyond Facebook is aspirational / stubbed.** The repo
  is **Facebook-first**. `IG/TikTok/Zalo/YT` and `AutoReelsVideoService` are not
  production features; Taobao/1688 exist only as resolver stubs.

---

## 4. Binding Conflicts: Diagram vs Repository Safety Rules

**This is the most important section.** Each conflict below records the diagram's
appearance and the **binding interpretation** that overrides it.

### Conflict A — Push/CDP diagram vs pull-based connector outbox

**Diagram appears to show:** `WebSocket Manager` pushing `Browser Commands` /
`WSS (Real-time)` to the `Chrome Extension`, and a `CDP Controller` issuing
`Platform-specific CDP Commands` — i.e. a server→extension **push** model.

**Binding interpretation (overrides the diagram):**

- THG AutoFlow execution **must remain pull-based**
  ([`CONNECTOR_STATE_MACHINE.md`](./CONNECTOR_STATE_MACHINE.md) §3).
- **The server must not push execution commands to the extension.** The connector
  **pulls** claimable work (`GET /connectors/outbox`); the server **claims** with
  row-level **CAS on `planned`** + a **lease**.
- This protects **double-claim prevention** (two tabs/devices on one account cannot
  both execute), **account safety**, and the **`human_required`** flow on
  login/checkpoint walls.
- A live screen/CDP channel may exist for *observability* and operate **within the
  extension's own pull loop**; it must not become a server-initiated execution
  push path.

### Conflict B — Temporal/BullMQ label vs Go modular monolith

**Diagram shows:** `Task & Workflow Engine (Temporal/BullMQ)`.

**Binding interpretation (overrides the diagram):**

- Treat this as a **logical workflow capability**, not a technology mandate.
- **Do not adopt BullMQ/Node** just because the diagram says so — it contradicts
  decision #1 in [`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md) (one Go
  modular monolith, not microservices; BullMQ is a Node runtime).
- Prefer the **Go transactional outbox + durable process managers**
  ([`TRANSACTIONAL_OUTBOX.md`](./TRANSACTIONAL_OUTBOX.md), roadmap Phase E) unless a
  future ADR explicitly approves an external engine.

### Conflict C — Content Gen Agent vs AI safety rules

**Diagram shows:** `Content Gen Agent`.

**Binding interpretation (overrides the diagram):**

- This means **text/copy generation** and **selection of real uploaded assets
  only**.
- **No AI image generation** (hard rule).
- **No fact invention** — every concrete claim must be grounded; missing grounding
  returns a typed `knowledge_gap`, never a hallucinated price/website/phone/proof
  ([`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md) §6).
- **No hidden-state asset fabrication.**

### Conflict D — Diagram omits the safety spine

**Diagram does not clearly show:**

- outbound coordination,
- approval gates,
- append-only action ledger / execution attempts,
- `human_required`,
- tenant isolation,
- connector lease / CAS,
- the transactional-outbox target.

**Binding interpretation (overrides the diagram):**

- These are **mandatory architecture elements**. Their absence from the diagram is
  **not** license to bypass them.
- Future refactors **must preserve them** even though the diagram does not depict
  them. They are defined in
  [`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md) §5,
  [`CONNECTOR_STATE_MACHINE.md`](./CONNECTOR_STATE_MACHINE.md),
  [`DATABASE_OWNERSHIP.md`](./DATABASE_OWNERSHIP.md), and
  [`TRANSACTIONAL_OUTBOX.md`](./TRANSACTIONAL_OUTBOX.md).

---

## 5. Correct Interpretation of the Diagram

```text
Use the diagram as capability/topology guidance.
Use docs/architecture as the implementation contract.
When diagram and contract disagree, contract wins.
```

Practical reading rule for any agent or engineer: **start from
`docs/architecture/*`**, then consult the diagram for *where the platform is
heading* (data tier, multi-platform, AI orchestration). Never derive a code change
from the diagram alone; cross-check the relevant binding doc and the controlled
zones in §9 first.

---

## 6. Target Module Structure

**Recommended direction (consistent with the standard):**

- modular monolith in a monorepo,
- service-oriented internal modules,
- ports/adapters at external boundaries,
- Python sidecars **only where justified** (sourcing/supplier analysis),
- **no premature microservices** (the one justified process split is `cmd/worker`).

Practical target tree (grounded in the actual repo; status marked per folder):

```text
cmd/
  scraper/                # API role (composition root)          — existing
  worker/                 # crawler role (composition root)      — existing
  agent/                  # connector/agent role                 — planned (no tracked package today)

internal/
  platform/               # service registry, tenancy root       — existing
  drivers/                # INBOUND adapters (outside → command)
    http/                 # REST/SSE transport (today server/*)  — proposed alias
    copilot/              # NL → intent → command                — existing
    telegram/             # webhook → command (today telegram/*) — proposed alias
    connector/            # extension pull endpoints (agent/*)   — proposed alias
  services/               # APPLICATION/workflow layer per vertical
    facebook/             # FB workflows (today spread)          — proposed
    taobao/               # future vertical                      — future
    supplier1688/         # future vertical                      — future
  automation/             # cross-vertical automation glue       — proposed
  connectors/             # connector bridge (store/connectors + browsergateway) — partial
  crawler/                # crawl engine (jobs/jobhandlers/store/crawl) — partial
  outbound/               # vertical-neutral coordination spine  — existing
  store/
    coordination/         # append-only ledger (single writer)   — existing
    outbound/             # queue/claim/lease/transition/policy   — existing
    connectors/           # connector/account binding            — existing
    postgres/             # PG dialect scaffolding (not runtime)  — partial
  ai/                     # PURE intelligence (models + stdlib)  — existing
  knowledge/              # KnowledgeOS grounding                 — existing
  events/                 # in-mem bus today; outbox target       — partial
  notifications/          # event sinks                          — existing
  auth/                   # auth                                 — existing
  session/                # session/browser-profile state        — existing
  models/                 # shared domain types (leaf)           — existing

frontend/                 # Next.js web UI                       — existing
local-connector-extension/# Chrome extension (Bridge)           — existing
services/
  supplier-analyzer/      # Python sidecar (Supplier Analyzer)   — future
docs/architecture/        # the binding standard                 — existing
```

**Per-folder boundary summary** (full rules in
[`MODULE_BOUNDARIES.md`](./MODULE_BOUNDARIES.md)):

| Folder | Owns | May import | Must NOT import | Status |
|---|---|---|---|---|
| `internal/platform` | registry, org/workspace shell, tenancy root | `models`, `store` (users/org), service contracts | any `services/*`, `drivers/*`, `ai` generators | existing/partial |
| `internal/drivers/*` | inbound translation only | application command ports, `models` | `store/<domain>` repos, sibling drivers, `services/*` internals | partial |
| `internal/services/*` | per-vertical workflow orchestration | shared primitives via ports, `ai`, `fburl`, `models` | sibling services, `drivers/*`, `internal/server` transport | proposed/future |
| `internal/ai` | pure classify/generate/repair | `models` + stdlib only | everything else | existing |
| `internal/outbound` + `store/outbound` | queue/claim/lease/transition/policy | `models`, `coordination`, `events`, `dbutil` | any `services/*`, `fburl`, `drivers/*`, `server` | existing |
| `internal/store/coordination` | append-only ledger (sole writer) | `models`, `dbutil` | service/workflow packages | existing |
| `internal/events` | durable bus/outbox/relay | `models`, `store` (outbox table) | `services/*`, `drivers/*`, business internals | partial |
| `services/supplier-analyzer` (Python) | supplier/image analysis | own deps; Go only via a versioned port | direct multi-tenant DB access | future |

---

## 7. Module Ownership and Boundaries

Risk = blast radius on production (org-5 crawler/comment/connector flows). "Tests
required" = what must be green/added **before** touching.

| Module | Layer | Responsibility | Current location | Target location | Risk | Tests required before refactor |
|---|---|---|---|---|---|---|
| Platform / workspace / membership / capabilities | Platform | registry, tenancy root, service status | `internal/platform`, `internal/workspace`, `store` (users/org) | same | Med | tenant-isolation guard green; workspace-switch tests |
| API gateway / HTTP handlers | Driver | REST/SSE transport | `internal/server/*` | `internal/drivers/http` (alias) | Med | handler char-tests for touched routes |
| Auth / session | Driver+Infra | login, JWT, cookies, session | `internal/auth`, `internal/session`, `internal/server/auth` | same | High | auth char-tests; cookie-flag behavior pinned |
| WebSocket / outbox / events | Infra | event bus → durable outbox | `internal/events`, `internal/runtime/events` | `internal/events` (+ outbox) | High | outbox write-in-tx, relay retry/idempotency, poison→dead |
| Facebook automation | Application | crawl/comment/inbox/post workflows | `cmd/scraper/*`, `internal/jobhandlers/facebook_crawl`, `internal/leadingest`, `internal/fburl` | `internal/services/facebook` (`fburl` stays a pure leaf) | High | `queueLeadOutreach` + direct-comment char-tests pinned first |
| Crawler cluster | App/Infra | distributed crawl, proxy, anti-bot | `internal/jobs`, `internal/jobhandlers`, `store/crawl`, `cmd/worker` | same | High | no-double-claim, crawl-submission char-tests |
| Browser connector / extension protocol | Infra | bridge, pairing, CDP, screenshots | `internal/browsergateway`, `internal/cdpclient`, `internal/server/agent`, `local-connector-extension` | `internal/connectors` + `drivers/connector` | High | pull/claim, lease-expiry, `human_required`, pairing tests |
| Comment automation | Application | comment planning/repair/verify | `cmd/scraper/outbound_actions.go`, `internal/ai/comment` | split: neutral→`outbound`, FB→`services/facebook`, pure→`ai` | High | comment forensics/reverify tests green |
| Lead ingestion | Domain/App | ingest, dedup, lead lifecycle | `internal/leadingest`, `store/leads` | `services/facebook` (ingest) + `leads` domain | High | lead lifecycle + attribution tests |
| AI planner / Copilot / LLM orchestration | Driver + pure | NL→command; pure generate/classify | `internal/drivers/copilot`, `internal/ai`, `internal/agentloop` | same (driver drops `*store.Store` in Phase G) | Med | routing char-tests; `AI_PURE` boundary |
| Store / database | Infra | SQLite now; PG dialect | `internal/store` (+ `postgres`, `dialect.go`) | same | High | migrator hardening, dialect tests; ETL per ADR-PR9 |
| Action ledger | Domain | append-only truth | `internal/store/coordination` | same | Critical | append-only invariant guard; never refactor casually |
| Observability | Infra | runtime feed, audit, logs | `internal/observability`, `internal/logstream`, `internal/server/observability` | same | Low | read-only projection tests |
| Future Taobao/1688 adapters | Application | sourcing verticals | `internal/platform/services/resolver/*` stubs | `internal/services/{taobao,supplier1688}` + `services/supplier-analyzer` (Py) | Low (new) | new-vertical contract tests; must not import `services/facebook` |

---

## 8. Data Flow: Correct Pull-Based Execution Model

The production-critical flow ("comment this lead") in its **correct** form:

```text
Dashboard / Copilot command
→ API Gateway / driver            (internal/server/*, internal/drivers/copilot)
→ auth / org validation           (internal/auth; org_id derived server-side)
→ application service              (queueLeadOutreach → target internal/services/facebook)
→ grounding + policy gate         (internal/ai/comment grounded by knowledge/brand;
                                   approval-required default)
→ outbound PLANNED row            (internal/store/outbound: execution_state='planned')
→ connector PULLS work            (GET /connectors/outbox — internal/server/agent)
→ lease / CAS claim               (row-level CAS on 'planned' + lease — claim.go/lease.go)
→ browser / extension executes    (local-connector-extension + browsergateway/cdpclient)
→ report sent back                (POST /connectors/outbox/:id/sent|failed)
→ verification                    (async reverify)
→ action ledger / execution attempts (internal/store/coordination — append-only)
→ DB / outbox / WebSocket / notification (events → notifications/telegram sink)
```

**This flow is, and must remain:**

- **pull-based** — the connector asks for work;
- **not server-push** — the server never pushes execution to the extension;
- **not direct extension command push** — no CDP/WebSocket execution command path;
- **not bypassing approval / `human_required`** — login/checkpoint walls keep the
  action `planned` and surface `human_required`; success counts only when
  **verified**.

This is the binding correction of the diagram's apparent push topology (Conflict A).

---

## 9. Controlled Zones

These zones carry production risk and must not be casually refactored. For each:
**why high risk**, **allowed safe work**, **prohibited casual refactor**.

| Zone | Why high risk | Allowed safe work | Prohibited casual refactor |
|---|---|---|---|
| Connector state machine | wrong account acts / double-post | add tests; codify states per the standard | changing readiness/claim semantics without char-tests |
| Browser extension protocol | breaks live customer pairing/execution | reliability fixes with tests (e.g. timeouts) | altering the pull/command contract or DTOs |
| Crawler runtime behavior | org-5 production crawl outages | local helper extraction (refactor-only) | changing claim/submission/scheduling behavior |
| Comment production behavior | wrong/forbidden outbound | grounded prompt fixes with tests | changing the outbound spine or gates |
| Classifier / scoring | misclassification → wrong action | pure-function extraction | changing decision thresholds without tests |
| Telegram / notifications | duplicate/lost operator alerts | render-only changes | adding business logic to the sink |
| Auth / session / security | account/tenant compromise | char-tested, reviewed changes | cookie/session/authz changes without security-review |
| WebSocket / outbox / backpressure / locking | lost events, ghost state | additive outbox alongside callbacks | replacing CAS/lease or event bus in place |
| Lead ingestion behavior | lead loss / cross-tenant bleed | tested additive changes | reordering ingest/dedup/lifecycle |
| DTO / wire contracts | extension/webhook incompatibility | add versioned fields + contract tests | breaking/renaming existing payload shapes |
| Database migrations / schema | broken production DB | additive idempotent migrations | editing shipped migrations; non-additive changes |
| Action ledger semantics | corrupts business truth | append-only additions | any UPDATE/DELETE of historical rows; multi-writer |

---

## 10. Migration / Refactor Roadmap

Aligned with and extending [`REFACTOR_ROADMAP.md`](./REFACTOR_ROADMAP.md)
(Phases A–D shipped; E is the keystone). This is the *diagram-reconciliation*
sequencing on top of that plan.

| Phase | Goal | Likely files touched | Risk | Validation | Rollback |
|---|---|---|---|---|---|
| **0 — Diagram reconciliation & module mapping** | record diagram vs standard conflicts (this document) | `docs/architecture/DIAGRAM_RECONCILIATION.md` | None | docs checks | delete doc |
| **1 — Ownership docs + import-boundary guard** | refresh ownership; add warn-only boundary rules (service-sibling, worker-no-transport, sidecar-no-DB) | `scripts/check_import_boundaries.sh`, `docs/architecture/*` | Low | `check_import_boundaries.sh` exit 0; `go build` | revert script (CI `\|\| true`) |
| **2 — Low-risk folder/module standardization** | move-only markers / package docs at target roots | `internal/*/doc.go` (markers only) | Low | full suite green; 0 new boundary warnings | mechanical revert |
| **3 — Service contracts & ports/adapters** | typed `CommandBus` + consumer-owned ports; driver drops `*store.Store` | `drivers/copilot`, `services/facebook`, `store/outbound`, `cmd/scraper/main.go` | Med | routing char-tests | remove new port (legacy path kept 1 cycle) |
| **4 — Controlled-zone refactors with characterization tests** | transactional outbox (Phase E) + connector hardening (Phase F) | `internal/events`, migration, `cmd/*` wiring, `store/connectors` | High | outbox tx/retry/poison; no-double-claim; lease-expiry | stop relay; drop table (no-op); callbacks remain |
| **5 — Future-service onboarding template (Taobao/1688)** | vertical module skeleton + Python sidecar port contract | `internal/services/taobao` skeleton, `services/supplier-analyzer` contract | Med (new) | new-vertical contract tests; `SERVICE_NO_SIBLING` | delete module |

Phase I (V2 Outbound PR2B breaking cleanup) stays **last and gated**, never
stacked on feature work — per the existing roadmap.

---

## 11. First 3 Safe PRs After This Document

### PR26A — Architecture diagram reconciliation docs (this PR)

- **Title:** `docs(arch): reconcile architecture diagram with module standard`
- **Branch:** `docs/pr26a-architecture-diagram-reconciliation`
- **Objective:** make the diagram a reconciled, first-class companion; record what
  it adds and the binding conflicts that override it.
- **Scope:** add `docs/architecture/DIAGRAM_RECONCILIATION.md`; optional one-line
  index reference in `ARCHITECTURE_STANDARD.md`.
- **Files likely touched:** this doc (+ optional one line in the standard).
- **Validation:** `git diff --check`; `python3 scripts/check_spec_registry.py`;
  `python scripts/check_file_size.py`.
- **Rollback:** delete the file.
- **Risk:** None (docs-only).
- **Must not change:** any code, schema, contract, CI guard, or the diagram image.

### PR26B — Import-boundary guard: diagram-driven rules (warn-only)

- **Title:** `chore(ci): add warn-only boundary rules for service-sibling + worker-transport + sidecar isolation`
- **Branch:** `chore/pr26b-import-guard-service-isolation`
- **Objective:** encode `SERVICE_NO_SIBLING`, `WORKER_NO_TRANSPORT`, and
  sidecar-no-direct-DB as **warn-only**, guarding the multi-service future before
  Taobao code lands.
- **Scope:** extend `scripts/check_import_boundaries.sh` only; CI step stays
  `|| true`.
- **Files likely touched:** `scripts/check_import_boundaries.sh`,
  `.github/workflows/ci.yml` (already non-blocking), a `REFACTOR_ROADMAP.md` note.
- **Validation:** `bash scripts/check_import_boundaries.sh` exits 0; `go build ./...`;
  `go vet ./...`.
- **Rollback:** revert the script.
- **Risk:** Very low (tooling, warn-only).
- **Must not change:** runtime code; any existing rule's pass/fail; 0 new failures.

### PR26C — Low-risk `doc.go` / package-marker standardization (move-only)

- **Title:** `refactor(arch): add doc.go boundary markers for target roots (move-only)`
- **Branch:** `refactor/pr26c-arch-doc-markers`
- **Objective:** make target boundaries physically visible via empty-package
  markers — the pattern Phase A.2 already used — with no logic moved.
- **Scope:** add `doc.go` markers under proposed roots lacking them; **no file
  moves, no import changes, no logic**.
- **Files likely touched:** a few new `doc.go` files + a `REFACTOR_ROADMAP.md` note.
- **Validation:** `go build ./...`; `go vet ./...`; `go test ./...`;
  `python scripts/check_file_size.py`; `bash scripts/check_import_boundaries.sh`
  (0 new warnings); `npm --prefix frontend run build`.
- **Rollback:** delete the marker files.
- **Risk:** Low (empty packages compile; no behavior).
- **Must not change:** migrations, connector/crawler/comment behavior, DTO/wire
  contracts, runtime wiring; no broad package move; no Sonar cleanup mixed in.

---

## Open Questions

1. **Vector store shape.** Is the KnowledgeOS vector store intended to be
   **pgvector inside Postgres**, or a **separate Milvus/Pinecone** service as the
   diagram shows? This changes the Phase-5 data-tier design and `ADR-PR9` follow-ups.
2. **Object storage.** Is **S3 / object storage** in scope, given the hard rule to
   use only real uploaded files? Where are uploads stored today (local FS)?
3. **Workflow engine.** Confirm we **reject Temporal/BullMQ** and treat the
   diagram's "Task & Workflow Engine" as the Phase-E Go transactional outbox +
   process managers (not a literal external engine).
4. **Push vs pull / CDP Controller.** Confirm the diagram's `CDP Controller` is the
   existing `internal/cdpclient` operating **within** the extension's pull loop
   (compatible), **not** a server-side execution push (incompatible — Conflict A).
5. **Multi-platform scope.** Which of `IG/TikTok/Zalo/YT` + `AutoReelsVideoService`
   are near-term vs aspirational? This decides whether the Platform-Adapter
   strategy generalizes now or stays FB-shaped until a second platform is funded.
6. **Python sidecar.** Is a Python sidecar approved for sourcing/supplier analysis,
   and should LLM orchestration stay in Go (recommended) or move to a Python
   LangGraph service?
7. **Content Gen Agent boundary.** Confirm it is **text/copy generation + selection
   of real uploaded media only** — no generated imagery, no fact invention
   (Conflict C).
