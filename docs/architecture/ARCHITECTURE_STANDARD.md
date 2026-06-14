# THG AutoFlow — Architecture Standard (v3)

**Status:** OFFICIAL STANDARD (target architecture). **Created:** 2026-06-14.
**Scope:** the whole platform — `Sale.thgfulfill.com` / THG AutoFlow.
**Authority:** this document and its siblings in `docs/architecture/` are the
binding architecture contract. Where an existing spec (`specs/RUNTIME_TOPOLOGY.md`,
`internal/store/DOMAINS.md`) describes the *current* code, this set describes the
*direction* the code must converge toward. On conflict, this set wins for **new**
work; existing code is migrated per `REFACTOR_ROADMAP.md`, never big-banged.

> This is a **documentation + tooling** standard. It changes no runtime behavior.
> It exists so that complex workflow features (direct-comment import continuation,
> multi-service Taobao/1688 automation, durable outbox) are built on a durable
> contract instead of ad-hoc wiring.

---

## 1. What we are building

THG AutoFlow is a **multi-service SaaS automation platform**, not a Facebook
tool. Facebook is the first *service module*; Taobao and 1688 are next. The
platform learns each organization's business and runs **observable, approval-
gated** browser automation through per-account connectors.

The architecture must therefore make these properties structural, not optional:

- **Tenant isolation** — every feature is `org_id`-scoped at the data layer.
- **Service pluggability** — a new vertical (Taobao) is a new *service module* on
  shared primitives, never a fork of the platform.
- **Durable side effects** — a "lead created → comment planned → comment posted"
  chain survives a process restart; it is not held in a Go channel.
- **Observability + safety** — automation is visible, outbound defaults to
  approval-required, login/checkpoint returns `human_required`, no secrets logged.

## 2. The seven decisions (binding)

1. **Modular Monolith.** One deployable per role (`cmd/scraper` API, `cmd/worker`
   crawler), many internal modules with hard import boundaries. Not microservices.
2. **Hexagonal / Ports & Adapters.** Domain + application logic depend on *ports*
   (interfaces), never on concrete infrastructure. Adapters implement ports;
   `main` wires them. See `PORTS_AND_ADAPTERS.md`.
3. **Transactional Outbox.** A critical cross-module side effect is written to an
   outbox table **in the same DB transaction** as the state change that caused it.
   A relay publishes it. See `TRANSACTIONAL_OUTBOX.md`.
4. **Durable internal event bus / process managers.** Cross-module workflows are
   driven by durable events + process managers (sagas), not by direct calls
   threaded through composition-root callbacks.
5. **Pull-based connector outbox.** Connectors (Chrome extensions) **pull** claimed
   work; the server claims with CAS/lease. No push, no double-claim. See
   `CONNECTOR_STATE_MACHINE.md`.
6. **Consumer-owned ports.** The module that *consumes* a capability owns its
   interface; the provider implements it. No global `internal/contracts` god-package.
7. **Explicit DB table ownership by module.** Every table has exactly one owner
   module that may write it; others read via projections. See `DATABASE_OWNERSHIP.md`.

## 3. Layer model

```
┌──────────────────────────────────────────────────────────────────────────┐
│  DRIVERS (inbound adapters) — translate the outside world into app calls   │
│    • drivers/copilot   (NL prompt → intent → application command)          │
│    • drivers/http      (REST/SSE handlers, internal/server/*)              │
│    • drivers/telegram  (webhook → application command)                     │
│    • drivers/connector (extension heartbeat/crawl-result/outbox endpoints) │
└───────────────┬──────────────────────────────────────────────────────────┘
                │ calls application commands / queries only
┌───────────────▼──────────────────────────────────────────────────────────┐
│  APPLICATION / WORKFLOWS — orchestrate domains; own no business rules      │
│    • per-service workflows (services/facebook, services/taobao, …)         │
│    • process managers (sagas) reacting to durable events                   │
│    depends on → domain ports + domain modules; NEVER on drivers            │
└───────────────┬──────────────────────────────────────────────────────────┘
                │ ports (interfaces owned by the consumer)
┌───────────────▼──────────────────────────────────────────────────────────┐
│  DOMAIN MODULES — business truth, one owner per truth                      │
│    leads · outbound(coordination) · knowledge · brand · identities ·       │
│    connectors · crawl/jobs · notifications · events(outbox)                │
│    pure rules: ai (intelligence), fburl (platform URL trust)               │
└───────────────┬──────────────────────────────────────────────────────────┘
                │ ports
┌───────────────▼──────────────────────────────────────────────────────────┐
│  INFRASTRUCTURE — store (SQLite), browsergateway, mailer, http client,     │
│  scheduler, event relay. Implements ports; depends on nothing above it.    │
└──────────────────────────────────────────────────────────────────────────┘
```

**Dependency rule (the one rule):** imports point **downward only**. Drivers →
Application → Domain → Infrastructure. Nothing imports a layer above it. Pure
modules (`ai`, `fburl`) sit at the bottom and import only `models` + stdlib.

## 4. Module catalogue (target → current code)

The target module names below are *logical*. Some already exist as packages;
others are aspirational and named so the boundary can be documented before the
move (see `REFACTOR_ROADMAP.md`). `MODULE_BOUNDARIES.md` gives the per-module rules.

| Logical module | Responsibility | Current code (as of 2026-06-14) |
|---|---|---|
| **platform** | service registry, workspace/org shell, tenancy root | `internal/platform/*`, `internal/workspace`, `internal/store` (users/org) |
| **drivers/copilot** | NL prompt → intent → application command | `internal/ai/agent*.go`, `internal/ai/intent_*.go`, `internal/ai/brain*.go` |
| **drivers/http** | REST/SSE transport | `internal/server/*` |
| **drivers/connector** | extension heartbeat/crawl-result/outbox endpoints | `internal/server/agent/*` |
| **drivers/telegram** | webhook → command | `internal/server/telegram`, `internal/telegram/*` |
| **services/facebook** | Facebook workflows (crawl, comment, inbox, post) | `cmd/scraper/*` orchestration, `internal/jobhandlers/facebook_crawl`, `internal/leadingest`, `internal/fburl` |
| **services/taobao**, **services/1688** | future sourcing verticals | resolver stubs in `internal/platform/services/resolver` |
| **ai** | PURE intelligence (classify, generate, repair) — no IO | `internal/ai/comment`, `internal/ai` generators |
| **knowledge** | KnowledgeOS assets, grounding | `internal/store/knowledge`, `internal/workspace_knowledge` |
| **brand** | brand/contact/persona identity for outbound grounding | `internal/server/contactprofile`, `staff_contact_profiles` |
| **outbound** | outbound coordination: queue, claim, lease, transition, policy | `internal/store/outbound`, `internal/store/coordination`, `cmd/scraper/outbound_actions.go` |
| **events** | durable event bus + transactional outbox + relay | `internal/events` (in-mem SSE today), `internal/runtime/events`, `runtime_events` table |
| **notifications** | per-org Telegram channel + in-app bell | `internal/telegram/control`, `internal/server/system/notifications.go`, `notifications` table |
| **identities** | Facebook accounts, browser profiles, sessions | `internal/store/identities`, `internal/session` |
| **connectors** | Chrome extension bridge, pairing, commands, screenshots | `internal/store/connectors`, `internal/browsergateway` |
| **crawl/jobs** | crawl jobs, scheduler, posts/groups/comments ingest | `internal/jobs`, `internal/store/crawl`, `internal/jobhandlers` |

## 5. Outbound coordination (the safety spine)

Every outbound action (comment / inbox / post) flows through ONE spine:

```
ActionContext → Readiness/PolicyGate → Plan(outbound_messages) → Claim(CAS/lease)
              → Connector pull → Execute → Report → Verify → Ledger(append-only)
```

- `execution_attempts` and `action_ledger` are **append-only** (coordination owns
  them). No raw writes outside `internal/store/coordination`.
- Telegram is an interface, not a logic path: Telegram commands go through the
  same `ActionContext → PolicyGate → Execution/Ledger` spine.
- `internal/ai/comment` (intelligence) repairs/validates content but never queues,
  claims, or executes — it has no store/connector/outbound import.

## 6. AI as pure intelligence

`internal/ai/comment` and the generators are **pure**: they take typed inputs
(business profile, lead content, catalog) and return typed decisions/text. They
import only `internal/models` + stdlib. They must not invent business facts
(price/website/email/proof) — missing grounding returns a typed `knowledge_gap`,
never a hallucination. The Copilot *driver* (`internal/ai/agent*.go`) is a
different thing: it is an inbound adapter and may orchestrate, but the
intelligence core stays pure.

## 7. How to use this standard

- **Adding a file:** classify its module (table in §4), check `MODULE_BOUNDARIES.md`
  for allowed/forbidden imports, run `scripts/check_import_boundaries.sh`.
- **Adding a table:** assign an owner in `DATABASE_OWNERSHIP.md`; only the owner
  writes it.
- **Adding a cross-module side effect:** if it is a *critical event* (§ in
  `TRANSACTIONAL_OUTBOX.md`), write it to the outbox in the same transaction —
  do not fan out via a composition-root callback.
- **Adding a vertical (Taobao):** new `services/taobao` module + resolver; reuse
  outbound/connectors/events primitives. Do not fork the platform.

## 8. Companion documents

| Doc | Defines |
|---|---|
| `MODULE_BOUNDARIES.md` | per-module responsibility + allowed/forbidden imports |
| `DATABASE_OWNERSHIP.md` | table → owner / readers / writers matrix |
| `TRANSACTIONAL_OUTBOX.md` | outbox table, envelope, relay, process managers |
| `PORTS_AND_ADAPTERS.md` | consumer-owned ports, composition root wiring |
| `CONNECTOR_STATE_MACHINE.md` | connector states, action lifecycle, pull/claim |
| `REFACTOR_ROADMAP.md` | staged path from current code to this standard |
| `CURRENT_CODE_AUDIT.md` | honest gap audit of the code as-of-now |
| `scripts/check_import_boundaries.sh` | warn-only static boundary guard |
