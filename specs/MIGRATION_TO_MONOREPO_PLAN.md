# Migration Plan: Monolith → apps / services / packages

**Status:** Deferred — execution not started. Revisit before Phase 0 to re-confirm assumptions (FB workflow, MQ choice, Postgres timing).
**Owner vision source:** [.claude/structurebase.md](../.claude/structurebase.md)
**Related plans:** [STRUCTURAL_REFACTOR_PLAN.md](STRUCTURAL_REFACTOR_PLAN.md), [PRODUCTION_DATABASE_MIGRATION_PLAN.md](PRODUCTION_DATABASE_MIGRATION_PLAN.md), [ROOT_ARCHITECTURE.md](ROOT_ARCHITECTURE.md)

## Confirmed defaults (2026-05-07)

User confirmed these three decisions for the plan as written. Re-validate at Phase 0 kickoff if more than ~1 month has passed:

1. **Module strategy:** `go.work` multi-module workspace (single repo, multiple Go modules under `apps/` + `services/` + `packages/`). Not multi-repo.
2. **Multi-platform language:** Python for Taobao + 1688 services (Playwright async ecosystem strength); Go remains for Facebook worker.
3. **Phase 0 timing:** Defer execution. Plan saved as reference; actual start happens later by explicit kickoff.

## Baseline (current state)

| Already in place | Still missing |
|---|---|
| [services/agent-brain/](../services/agent-brain/) Python sidecar — polyglot precedent exists | `apps/`, `packages/`, `infrastructure/` directories |
| [frontend/](../frontend/) Next.js | Single flat `go.mod` — no workspace yet |
| 30 packages under [internal/](../internal/), 3 entrypoints under [cmd/](../cmd/) | In-process job queue ([internal/jobs/](../internal/jobs/) + SQLite) — no real MQ |
| [local-connector-extension/](../local-connector-extension/) | Facebook-only — Taobao/1688 services do not exist |
| Ad-hoc [docker/](../docker/), [deploy/](../deploy/) | No IaC standard |

## Cross-cutting principles

1. **Each phase ships independently — no half-built states.** Code on `main` must always boot and serve traffic.
2. **Refactor structure first, change behavior never (within structural phases).** Phases 1–3 are pure code-movement. Existing tests must continue to pass with no behavioral change.
3. **One bundled commit per phase** (per user preference for bundled refactors). Do not split structural moves into many small PRs.
4. **Verify with existing tests** — do not write new tests purely to validate refactors. Tests we have are the contract.

---

## Phase 0 — Skeleton + go.work + path moves

**Goal:** Create the target skeleton without touching any code.

**Scope:**

- Create `apps/`, `services/`, `packages/`, `infrastructure/` (services/ already exists).
- Convert root `go.mod` → `go.work` (multi-module workspace). Do not create child modules yet — only flip the root.
- Path moves:
  - `frontend/` → `apps/web-dashboard/`
  - `local-connector-extension/` → `apps/connector-extension/`
- Update path references in: [Dockerfile](../Dockerfile), [docker-compose.yml](../docker-compose.yml), [Makefile](../Makefile), `.github/workflows/*`, [package.json](../package.json) scripts, deploy scripts.

**Risk:** Low — pure renames. Main hazard is missed `frontend/` path references in CI.

**Verify:** `go build ./...`, `npm --prefix apps/web-dashboard run build`, `docker-compose build`.

---

## Phase 1 — Extract `packages/core-*` (mechanical refactor)

**Goal:** Four reusable Go modules that stand alone, so future Go services can import them without dragging in the monolith.

**Scope (4 child Go modules under `go.work`):**

| Package | Pulled from | Public surface |
|---|---|---|
| `packages/core-browser` | [internal/browser/](../internal/browser/), [internal/runtime/fingerprint.go](../internal/runtime/fingerprint.go), `internal/runtime/stealth*`, [internal/identity/](../internal/identity/) | `Browser.Launch(opts)`, `Stealth(...)`, proxy rotation |
| `packages/core-database` | [internal/store/](../internal/store/), [internal/models/](../internal/models/) | Store interface + SQLite implementation. Schema/migrations stay in [db/](../db/) |
| `packages/core-logger` | [internal/observability/](../internal/observability/), [internal/logstream/](../internal/logstream/) | Structured logger + Prometheus metrics |
| `packages/core-queues` | [internal/jobs/](../internal/jobs/), [internal/events/bus.go](../internal/events/) | `Producer`, `Consumer` interfaces + in-process implementation (pluggable) |

**Critical:** `core-queues` only defines interfaces and keeps the existing in-process implementation. Real MQ backend lands in Phase 4.

**Risk:** Medium — many import paths change. Some `internal/*` packages couple too tightly to concrete `*store.Store` types (e.g. [internal/leadingest/](../internal/leadingest/)) and must be loosened to accept interfaces. This is good decoupling, not overhead.

**Verify:** Full `go test ./...` passes with no behavioral change.

---

## Phase 2 — `apps/api-gateway`

**Goal:** Gateway HTTP/Fiber + tenant logic out of `cmd/scraper`.

**Scope:**

- `cmd/scraper/` + [internal/server/](../internal/server/) + [internal/auth/](../internal/auth/) + [internal/ai/](../internal/ai/) + [internal/skills/](../internal/skills/) + [internal/workspace/](../internal/workspace/) → `apps/api-gateway/`
- Becomes its own `go.mod`, importing `packages/core-*`.
- Telegram bot ([internal/telegram/](../internal/telegram/)) follows the gateway (same user-facing tier).

**Risk:** Medium — `internal/ai` is imported in many places. May eventually need its own `packages/core-ai`, but **not in this phase** — keep scope tight.

**Verify:** Server boots, `/api/auth/login`, `/api/leads`, `/api/agent/*` all return 200.

---

## Phase 3 — `services/fb-automation-worker`

**Goal:** Worker fully separated from gateway, communicates only via queue (+ shared DB read).

**Scope:**

- `cmd/worker/` + `cmd/agent/` + [internal/jobhandlers/](../internal/jobhandlers/) + [internal/agentloop/](../internal/agentloop/) + [internal/leadingest/](../internal/leadingest/) + [internal/browsergateway/](../internal/browsergateway/) + [internal/livesession/](../internal/livesession/) → `services/fb-automation-worker/`
- Worker subscribes via `core-queues.Consumer` (still in-process backend in this phase).
- Gateway publishes tasks via `core-queues.Producer`. The two sides communicate **only** through queue + DB; no direct shared Go structs.

**Risk:** **High** — gateway and worker currently share state through `internal/store` calls. Need to clearly classify each interaction as either "command via queue" or "read shared DB". Some synchronous calls (live browser session control) cannot be made async cleanly in one move.

**Decision point:** Live browser session control is currently synchronous. Either keep a small internal gRPC for session control during this phase, or fully async it. **Recommendation:** keep gRPC for session control here; everything else goes through queue. Full async lands in Phase 4.

**Verify:** End-to-end smoke test — Chrome Extension crawl → gateway → queue → worker → leads visible on dashboard.

---

## Phase 4 — Real message queue (Redis Streams)

**Goal:** Stop using SQLite as a queue. Move to Redis Streams.

**Why Redis Streams over Kafka / RabbitMQ:**

- Redis is already in most deploys (cache, rate limit). No new operational component.
- Consumer groups + ack + replay are sufficient for automation workloads up to ~1M tasks/day.
- Switch to Kafka only when needed (>100k msg/s sustained or cross-region durability). Not needed yet.

**Scope:**

- Implement `RedisProducer` / `RedisConsumer` in `packages/core-queues`.
- Idempotency stays at the DB layer — `task_leads` and `outbound_messages` already enforce UNIQUE constraints. Redis is transport only.
- **Dual-write for one release:** both in-process and Redis backends run side-by-side, log mismatches.
- After verification: drop in-process backend, drop SQLite job table.

**Risk:** **Highest in the entire plan.** Recommend executing only after Phases 1–3 have been on `main` and stable for at least 1–2 weeks.

**Verify:** Worker scales 1 → 3 instances. Each task runs exactly once. No lost tasks when killing a worker mid-task.

---

## Phase 5 — Multi-platform (Taobao + 1688)

**Goal:** Two new services that reuse `core-queues` via a JSON contract.

**Scope:**

- `services/taobao-scraper-worker/` (Python + Playwright async)
- `services/1688-order-worker/` (Python + Playwright async)
- Add `packages/core-contracts/` with JSON schema for the task envelope. Single schema covers all platforms: `{platform, action, payload}`.

**Decision point:** Reuse Go `core-browser` over RPC vs. rewrite browser layer in Python?

- **Recommendation:** rewrite in Python for Taobao/1688. The anti-detect playbook differs significantly from Facebook (different fingerprint heuristics, Chinese sites use different bot detection). Reusing FB's Go logic would introduce more friction than benefit. Keep Go `core-browser` for the Facebook worker.

**Risk:** Medium — new services don't touch existing code. Main risk is contract drift between Go gateway and Python workers.

**Verify:** End-to-end one Taobao crawl task → result row appears in shared DB → dashboard renders it.

---

## Phase 6 — Infrastructure as code

**Goal:** A single command brings the full stack up locally.

**Scope:**

- `infrastructure/docker/docker-compose.dev.yml` — api-gateway + fb-worker + taobao-worker + 1688-worker + redis + postgres + web-dashboard.
- `infrastructure/k8s/` — Helm chart or raw manifests for production.
- CI: per-service build matrix. Only rebuild what changed.

**Verify:** `make up` brings the full stack on a fresh machine. PR previews auto-deploy.

---

## Cross-cutting (anytime, prioritise immediately after Phase 1)

### No-hardcoding pass

Extract Facebook CSS selectors from [internal/browser/actions.go](../internal/browser/actions.go) and [local-connector-extension/content/crawl.js](../local-connector-extension/content/crawl.js) into a config file (YAML or JSON). Move FB endpoint URLs to env. Credentials are already in DB. This is hardening — not tied to any phase.

### SQLite → Postgres

A separate plan, not bundled with this structural refactor. Can run any time after Phase 1, but recommended after Phase 4 (real MQ in place — horizontal scale story is cleaner with Postgres in front of Redis Streams).

---

## Effort + recommended order

| Phase | Effort | Risk | Can parallelise with |
|---|---|---|---|
| 0 | 1 day | Low | — |
| 1 | 1 week | Med | After Phase 0 |
| 2 | 3–5 days | Med | After Phase 1 |
| 3 | 1–2 weeks | High | After Phase 2 |
| 4 | 1 week | **Highest** | After Phase 3 stable for 1–2 weeks |
| 5 | 2–3 weeks per service | Med | After Phase 4 |
| 6 | 1 week | Low | Parallel with Phase 5 |
| No-hardcoding | 2–3 days | Low | After Phase 1 |

**Total:** ~2 months sequential, ~6 weeks if Phase 5 and 6 run in parallel.

---

## Open questions to revisit before kickoff

- Is the Facebook worker still the highest-traffic path when this kicks off, or has Taobao/1688 priority shifted? Affects whether to bring up new services before or after the FB queue swap.
- Has the Postgres migration already happened by the time this starts? Affects what Phase 1 `core-database` exports as its public interface (raw SQL vs. ORM-style).
- Has Redis already been added to the stack for another reason? If yes, Phase 4 risk drops because operational expertise already exists.
- Is the Chrome Extension content script under `apps/connector-extension/` going to stay extension-shaped, or pivot to a native Chrome connector with a different distribution path? Affects Phase 0 path choice.
