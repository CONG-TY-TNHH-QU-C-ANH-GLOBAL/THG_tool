# ADR — PR9 Data Platform

**Status:** ACCEPTED (documentation only — PR9 ships NO runtime/data code).
**Date:** 2026-06-21.
**Track:** KnowledgeOS / Data Platform (infrastructure).
**Companion of:** [`DATABASE_OWNERSHIP.md`](./DATABASE_OWNERSHIP.md),
[`ARCHITECTURE_STANDARD.md`](./ARCHITECTURE_STANDARD.md).
**Builds on (does not duplicate):**
[`specs/PRODUCTION_DATABASE_MIGRATION_PLAN.md`](../../specs/PRODUCTION_DATABASE_MIGRATION_PLAN.md)
(target architecture + schema-conversion rules) and
[`specs/POSTGRES_COMPAT_PLAN.md`](../../specs/POSTGRES_COMPAT_PLAN.md)
(dialect layer — PR-1/2/3 already shipped for the Knowledge OS domain).

---

## Context

The product runs on SQLite (`modernc.org/sqlite`) today. The dual-target dialect
layer already exists (`internal/store/dialect*.go`, migration runner, PG baseline
for Knowledge OS) — see `POSTGRES_COMPAT_PLAN.md`. What is **not** yet decided is
*how the existing SQLite data crosses to PostgreSQL at cutover*. That is the gap
this ADR closes.

This ADR is **documentation only**. PR9:

- does NOT implement ETL or migration code,
- does NOT modify runtime DB initialization,
- does NOT add a Postgres adapter or driver wiring,
- does NOT change worker / connector / outbound behavior.

It records the **strategy** so that a later, intentional migration PR (and the
PR10/PR11 JSONB work) is built against an agreed decision rather than improvised
at cutover time.

---

## Decision: Local development infrastructure (PR9 deliverable)

PR9 adds a **local development data platform** (PostgreSQL + Redis) so engineers
can build and test the future Postgres path without touching the live runtime.

- **PR9 adds dev infrastructure only — there is NO runtime switch.** The app's
  boot path, DB initialization, and worker/connector/outbound behavior are
  unchanged.
- **SQLite remains the current default** (`DB_PATH=data/scraper.db`). It is the
  only database the application runtime reads today. `DB_DRIVER` is documented in
  the dev env example but is **not wired into runtime code** in this PR.
- **PostgreSQL is the future source of truth** (per
  `PRODUCTION_DATABASE_MIGRATION_PLAN.md`), reached later via an intentional,
  feature-flagged cutover — not in PR9.
- **Redis is ephemeral only** — presence / rate-limit / cache acceleration. It is
  **never** a durable source of truth for task / queue / ledger / proof / policy
  data. The dev Redis runs without `appendonly` and without a volume to reinforce
  this; even if `appendonly` were enabled later, Redis stays non-authoritative for
  those domains.
- **Docker Compose is local development only**, not a production deployment. It
  lives at [`deploy/dev/docker-compose.yml`](../../deploy/dev/docker-compose.yml),
  deliberately separate from the production root `docker-compose.yml`, and binds
  Postgres/Redis to `127.0.0.1` only.
- **Real production secrets must not be committed.** The dev env example
  ([`deploy/dev/.env.example`](../../deploy/dev/.env.example)) ships only safe
  local-only defaults; the actual `deploy/dev/.env` is git-ignored.

---

## Decision: Data migration strategy

The future PostgreSQL cutover will **NOT copy 100% of SQLite blindly**. Data is
classified into three tiers, each with a different handling rule. Tier examples
are grounded in the real ownership map in
[`DATABASE_OWNERSHIP.md`](./DATABASE_OWNERSHIP.md).

### Tier 1 — Core data: migrate 100%

Core data is durable product state and must be migrated from SQLite to PostgreSQL.

Examples (this codebase):

- `users`, `organizations`, `org_invites` — tenant identity
- workspace / membership / role / permission rows
- service / workspace configuration
- `accounts` and connector/account binding configuration (`agent_tokens`,
  `extension_policies`)
- `action_policies` (domain-agnostic outbound policy)
- `staff_contact_profiles`, `kpi_config`
- billing / subscription / customer records **if present** at migration time
- durable system settings (e.g. durable `user_context` config keys, **not**
  transient continuation rows — see Tier 3)

Rules:

- Must be migrated with **deterministic ETL** (same input → same output; no
  wall-clock or random ordering effects).
- Must **preserve tenant/org ownership** — every `org_id` association carried
  exactly; no cross-tenant bleed; foreign-org rows never reparented.
- Must **preserve IDs**, or provide an explicit **ID-mapping table** if IDs change
  (PG `BIGSERIAL` sequences must be reseeded so new inserts never collide).
- Must **validate row counts and referential integrity** after migration (per-table
  count parity + FK / orphan checks).
- Must have a **rollback / archive plan** (the source SQLite file is retained
  read-only until verification + retention sign-off — see ETL policy).

### Tier 2 — Ledger / proof / audit data: migrate selectively or archive

Durable history, but it does not all need to be **hot** in PostgreSQL.

Examples (this codebase):

- completed `outbound_messages` (terminal `execution_state` / `verification_outcome`)
- `execution_attempts` (append-only)
- `action_ledger` (append-only; corrections are `engagement_revoked` rows)
- proof / report / audit rows: `connector_screenshots`, `comment_verification_audit`,
  `audit_logs`, `prompt_logs`
- historical automation results / classification logs

Default strategy:

- Migrate **recent** ledger/proof data (e.g. **last 3–6 months**) to PostgreSQL,
  unless product / legal / audit requirements demand more.
- Keep older SQLite data as a **read-only cold archive**.
- Do **not** discard old SQLite files until production verification and the
  retention policy are approved.

Rules:

- Recent records migrated to PostgreSQL must **preserve audit meaning** — the
  append-only invariant holds in PG too (insert-only; corrections are new rows,
  never UPDATE/DELETE of historical truth).
- The archive must be **documented and restorable/readable** (what file, what
  schema version, how to query it).
- Do **not** mix archive migration with the queue cutover (Tier 3 drain is a
  separate operation with its own runbook step).
- Do **not** migrate stale transient lease state as durable truth (lease columns
  on `outbound_messages` are Tier 3, even though the row itself is Tier 2 when
  terminal).

### Tier 3 — Ephemeral / queue / in-flight data: do NOT migrate

Ephemeral execution state must not be copied into PostgreSQL.

Examples (this codebase):

- pending `jobs` rows still waiting to be claimed (crawl job queue)
- executing / in-flight task state; `connector_commands` not yet acked
- `claimed_by`, `claimed_at`, `lease_expiry`, in-flight `execution_id`
  (legacy lease columns on `outbound_messages`; reverify leases on `comment_reverify`)
- retry locks / `retry_after` backoff windows
- WebSocket / SSE sessions
- connector presence / heartbeat / `connector_screenshots` liveness
- `connector_pairing_codes` (one-time, expiring secrets)
- cache / rate-limit counters

Default strategy: **drain, not ETL.**

#### Drain plan

1. **Stop ingress into SQLite.**
   - New outbound tasks are no longer created in SQLite.
   - New tasks go to PostgreSQL only **after** the feature-flagged cutover point.

2. **Drain SQLite.**
   - Existing SQLite workers continue processing pending SQLite tasks.
   - No new SQLite tasks are admitted.

3. **Verify empty queue.**
   - SQLite pending/executing task count reaches **zero**.
   - Stale leases are reset or finalized **according to existing behavior** (the
     lease-expiry safety net) before shutdown — no new finalization semantics are
     invented for the cutover.

4. **Sunset SQLite workers.**
   - Turn off workers connected to SQLite.
   - PostgreSQL becomes the active durable task backend.

5. **Archive.**
   - Keep the SQLite file as a **read-only archive** until the retention period
     expires.

Rules:

- Do **not** copy in-flight leases into PostgreSQL.
- Do **not** duplicate pending tasks into PostgreSQL while SQLite workers can still
  process them.
- Do **not** allow both SQLite and PostgreSQL workers to claim the same logical
  task (single active backend at any instant; the feature flag is the switch).
- Cutover must use **feature flags** and an **operational runbook**.

---

## Decision: ETL tooling policy

Physical data migration must be implemented as a **standalone ETL tool/script**,
not embedded into the main runtime path.

Rules:

- The ETL script may live under `scripts/` (or another repo-approved migration
  tooling location).
- ETL connects to the **SQLite source** and the **PostgreSQL target**.
- ETL is run **intentionally by an operator** during migration.
- ETL must be **idempotent or safely resumable** (re-running after a partial run
  must not duplicate Tier 1 rows or corrupt sequences).
- ETL must produce a **report**:
  - rows read
  - rows inserted
  - rows skipped
  - errors
  - checksum / count validation
- ETL must **not** become part of normal app startup.
- ETL must **not** run automatically in production.
- ETL must **not** be mixed with worker runtime behavior.

> Note: this supersedes the throwaway "`pg_dump` / `pgloader` one-shot" remark in
> the existing PG specs. A tiered migration (copy Tier 1, window Tier 2, drain
> Tier 3) cannot be a blind dump — it needs a purpose-built, reportable tool.

---

## Future requirement: JSON / JSONB migration inventory (PR10 / PR11)

Before designing any PostgreSQL `JSONB` columns, **PR10/PR11 must first inventory
the SQLite TEXT columns that actually contain JSON.** Do **not** guess exact
columns in PR9 — the current schema must be inspected at inventory time.

The future inventory should, per column, identify:

- table name
- column name
- JSON shape example
- owner / domain (per `DATABASE_OWNERSHIP.md`)
- whether it should become **`JSONB`**, **normalized relational columns**, or
  **remain TEXT**
- required indexes if `JSONB` (e.g. GIN)
- validation rules
- backward-compatibility concerns

**Risk note:** do **not** blindly convert every SQLite TEXT column containing
JSON-like data to PostgreSQL `JSONB`. Some fields are **opaque external payloads**
and should remain archived/raw (e.g. connector `connection_config`, evidence
blobs); others are **queryable product state** and deserve `JSONB` or relational
normalization. `POSTGRES_COMPAT_PLAN.md` §3.4 already starts this cost/benefit
table for the Knowledge OS columns — PR10/PR11 extends it across all domains.

---

## What PR9 deliberately does NOT do

- No ETL implementation. No migration implementation.
- No change to runtime DB initialization (`internal/store/store.go`, migrator).
- No Postgres adapter / driver import added.
- No worker / connector / outbound behavior change.
- No JSON→JSONB column conversion (PR10/PR11, after inventory).

This ADR is the agreed strategy only; each numbered migration step is its own
later, intentional PR.

---

## PR9 completion report (ADR + dev infrastructure)

- **Files changed:**
  - `docs/architecture/ADR-PR9-DATA-PLATFORM.md` (this ADR)
  - `deploy/dev/docker-compose.yml` (new — dev PostgreSQL + Redis)
  - `deploy/dev/.env.example` (new — dev-only env example)
- **Over 200 lines?** No. Markdown/YAML config — not production code; the
  `check_file_size.py` 200-line rule targets code files, not docs/specs/compose.
- **Large legacy file touched?** No. The production root `docker-compose.yml` and
  root `.env.example` were deliberately left untouched (no production hijack).
- **Logic extracted / reused?** N/A (docs + dev infra); references existing
  `PRODUCTION_DATABASE_MIGRATION_PLAN.md`, `POSTGRES_COMPAT_PLAN.md`, and
  `DATABASE_OWNERSHIP.md` instead of duplicating their content.
- **Runtime behavior changed?** No. **Schema/migration changed?** No.
  **DB adapter added?** No. **SQLite still default?** Yes.
- **Intentional exceptions:** none.
- **Tests/builds run:** no Go/FE code changed, so app tests not required;
  `docker compose config` validates the dev compose (see PR description).
- **Behavior-changing or refactor-only?** Neither — documentation + dev infra only.

### Required confirmations

- ✅ ADR documents **core-data-only** (Tier 1) migration strategy.
- ✅ ADR documents **selective ledger/proof migration + cold-archive** (Tier 2)
  strategy.
- ✅ ADR documents the **drain strategy** for queue / in-flight data (Tier 3).
- ✅ ADR states **ETL must be standalone**, operator-run, reportable, and **not**
  runtime code.
- ✅ ADR includes the **future JSON/JSONB inventory requirement** (PR10/PR11) with
  the do-not-blindly-convert risk note.
