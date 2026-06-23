# Production Database Migration Plan

Last updated: 2026-05-04

Audience: Claude Code, Codex, and future engineering agents.

This repository currently uses local SQLite (`modernc.org/sqlite`) as the main
application database and scheduler store. SQLite is acceptable for MVP/local
development, but it is not the correct production foundation for a multi-tenant
Facebook Sales Intelligence automation service.

The production target is:

> PostgreSQL as the primary system-of-record database, with optional Redis for
> ephemeral presence/rate-limit/queue acceleration, and object storage for files.

Do not scale the product by adding more SQLite-specific behavior.

## 1. Why SQLite Must Not Be The Production Target

The product needs to support:

- many organizations
- many users/staff per organization
- many Facebook accounts per workspace
- many Chrome Extension connector devices
- recurring crawl jobs every 30 minutes or faster
- outbox execution, cooldowns, and conversation state
- audit trails for every agent decision/action
- Data Private files, connector summaries, and sales voice examples
- future pgvector-style retrieval for org memory

SQLite becomes risky because:

- one-file storage is hard to scale horizontally
- write concurrency is limited even with WAL
- background worker + API + connector heartbeat writes can contend
- online migrations are fragile
- replication/failover/backups are not production-grade enough for this service
- multi-tenant query isolation and reporting will grow beyond simple local SQL

## 2. Production Data Architecture

Use this split:

```text
PostgreSQL
  - organizations, users, roles, invites
  - Facebook accounts and connector/device ownership
  - browser sessions and screenshots metadata
  - scheduler jobs and app tasks
  - leads, posts, comments, inbox threads
  - outbound queue and guardrails
  - skill registry overrides and skill executions
  - Data Private summaries and Sales Voice Profile
  - audit logs and action decisions

Object storage (S3/R2/MinIO)
  - uploaded private files
  - org logos/avatars
  - browser screenshots if retained
  - media assets

Redis (optional but recommended)
  - online connector presence cache
  - websocket/session fanout
  - rate-limit counters
  - short-lived pairing codes
  - distributed locks if Postgres advisory locks are not enough

pgvector or external vector store (phase later)
  - semantic retrieval for business memory, sales examples, private docs
```

SQLite can remain only for:

- local developer mode
- test fixtures
- one-command demo mode

Production must use PostgreSQL.

## 3. Current SQLite Coupling

The codebase is currently coupled to SQLite in several ways:

- `internal/store/store.go`
  - hardcodes `sql.Open("sqlite", ...)`
  - contains many `INTEGER PRIMARY KEY AUTOINCREMENT`
  - uses SQLite `DATETIME`, `CURRENT_TIMESTAMP`, `datetime('now')`
  - uses `LastInsertId()`
  - contains migrations as raw inline SQL

- `internal/jobs/store.go`
  - hardcodes SQLite
  - uses `INSERT OR IGNORE`
  - uses SQLite datetime expressions for retry backoff
  - serializes writes with `SetMaxOpenConns(1)`

- Other store files
  - use `?` placeholders
  - use `INSERT OR IGNORE`
  - use SQLite date functions
  - rely on `LastInsertId()`

Postgres migration is not a search-and-replace. It needs a small database
abstraction and a real migration strategy.

## 4. Target Go Database Layer

### 4.1 Config

Add:

```text
DATABASE_URL=postgres://user:pass@host:5432/thg?sslmode=require
DB_DRIVER=postgres
```

Keep `DB_PATH` only for local SQLite fallback.

Recommended config behavior:

- `APP_ENV=production` requires `DATABASE_URL`.
- Local dev defaults to SQLite if `DATABASE_URL` is empty.
- Tests can use SQLite initially, then add Postgres integration tests.

### 4.2 Driver

Use `pgx` through `database/sql` first:

```go
_ "github.com/jackc/pgx/v5/stdlib"
sql.Open("pgx", databaseURL)
```

This keeps the first migration smaller. Later, direct `pgxpool` can be used for
advanced Postgres features.

### 4.3 Dialect Layer

Introduce a small dialect helper:

```go
type Dialect string

const (
    DialectSQLite   Dialect = "sqlite"
    DialectPostgres Dialect = "postgres"
)

type Store struct {
    db      *sql.DB
    dialect Dialect
}
```

Helpers:

- `Placeholder(n int) string` -> `?` for SQLite, `$1` for Postgres
- `NowSQL()` -> `CURRENT_TIMESTAMP` or `NOW()`
- `InsertReturningID(...)` pattern for Postgres
- `IsDuplicateColumnError(err)` by dialect
- `IsUniqueViolation(err)` by dialect

Avoid scattering dialect conditionals across business logic.

## 5. Migration Tooling

Move migrations out of giant inline strings.

Recommended structure:

```text
internal/db/migrations/
  000001_init.sql
  000002_auth.sql
  000003_browser_runtime.sql
  000004_outbound_guardrails.sql
  000005_data_private.sql
  000006_skills.sql
  000007_sales_voice.sql
```

Use a migration tool such as:

- `github.com/pressly/goose/v3`
- or `github.com/golang-migrate/migrate/v4`

First slice can keep existing inline SQLite migrations and add Postgres
migrations side-by-side. Do not try to perfectly convert everything in one PR.

## 6. Schema Conversion Rules

SQLite -> Postgres:

```text
INTEGER PRIMARY KEY AUTOINCREMENT -> BIGSERIAL PRIMARY KEY
DATETIME                          -> TIMESTAMPTZ
TEXT DEFAULT '{}'                 -> JSONB DEFAULT '{}'::jsonb where structured
INTEGER boolean flags             -> BOOLEAN where safe
INSERT OR IGNORE                  -> INSERT ... ON CONFLICT DO NOTHING
LastInsertId()                    -> RETURNING id
datetime('now', '+N seconds')     -> NOW() + INTERVAL 'N seconds'
? placeholders                    -> $1, $2, ...
```

Keep timestamps in UTC at the application boundary.

## 7. Scheduler / Jobs In Postgres

The scheduler should use Postgres row locks:

```sql
UPDATE scheduler_jobs
SET status = 'running',
    claimed_by = $1,
    claimed_at = NOW(),
    updated_at = NOW()
WHERE id = (
  SELECT id
  FROM scheduler_jobs
  WHERE status = 'pending'
    AND (retry_after IS NULL OR retry_after <= NOW())
  ORDER BY created_at ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
RETURNING ...
```

This is safer than SQLite's single-writer pattern and allows multiple worker
processes.

## 8. Outbound Queue In Postgres

The outbound queue is a critical production table.

Must preserve:

- org scope
- account scope
- dedup per target
- cooldown
- thread state
- status transitions

Use transactions and unique partial indexes:

```sql
CREATE UNIQUE INDEX uq_outbound_active_target
ON outbound_messages(org_id, type, target_url)
WHERE status IN ('draft', 'approved');
```

For auto-execution:

- Go decides status via `QueueOutboundForOrg`.
- Chrome Extension only executes `approved`.
- Chrome Extension marks `sent` or `failed`.

No AI prompt can flip organization auto mode directly.

## 9. Data Private and Sales Voice

Postgres should become the system of record for:

- private file metadata
- data source metadata
- business memory summaries
- sales voice examples
- sales voice profile
- skill executions
- agent action decisions

Object files go to S3/R2/MinIO, not local disk in production.

Suggested future tables:

```sql
sales_voice_examples (
  id BIGSERIAL PRIMARY KEY,
  org_id BIGINT NOT NULL,
  type TEXT NOT NULL,
  label TEXT NOT NULL,
  content TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT '',
  created_by BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

agent_action_decisions (
  id BIGSERIAL PRIMARY KEY,
  org_id BIGINT NOT NULL,
  account_id BIGINT NOT NULL DEFAULT 0,
  lead_id BIGINT NOT NULL DEFAULT 0,
  action TEXT NOT NULL,
  decision TEXT NOT NULL,
  confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
  guardrails JSONB NOT NULL DEFAULT '{}'::jsonb,
  message TEXT NOT NULL DEFAULT '',
  outbound_id BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## 10. Migration Phases

### Phase 0 — Stop Expanding SQLite Assumptions

- Add this document to `AGENTS.md`.
- Do not add new SQLite-only features unless wrapped by dialect helpers.
- New tables should have Postgres-compatible schema notes.

### Phase 1 — Add Postgres Connection Path

- Add `DATABASE_URL` and `DB_DRIVER`.
- Add `internal/db` opener.
- Add dialect field to `store.Store` and `jobs.Store`.
- Keep SQLite dev fallback.

### Phase 2 — External Migration Files

- Add Postgres migrations.
- Keep SQLite migrations for dev.
- Add migration command in CI.

### Phase 3 — Convert Core Tables First

Priority order:

1. organizations/users/auth/refresh_tokens
2. accounts/browser_sessions/agent_tokens/connector_screenshots
3. scheduler_jobs/app_tasks/task_leads
4. leads/posts/comments/groups
5. outbound_messages/conversation_threads/conversation_messages
6. private_files/data_sources/user_context
7. skill_executions/org_skills

### Phase 4 — Production Cutover

- Create managed Postgres database.
- Run migrations.
- Build one-time SQLite -> Postgres export/import script.
- Put production in maintenance mode.
- Import data.
- Verify row counts and key constraints.
- Deploy backend with `DATABASE_URL`.
- Keep SQLite backup artifact read-only.

### Phase 5 — Scale Hardening

- Add connection pool settings.
- Add slow query logs.
- Add tenant-heavy indexes.
- Move file storage to object storage.
- Add Redis only where needed.
- Add Postgres backup/restore runbook.

## 11. CI Requirements

Before production migration is considered done:

- `go test ./...` with SQLite local mode
- Postgres integration tests for:
  - auth
  - account identity
  - connector pairing/heartbeat
  - job claim concurrency
  - outbound queue dedup/cooldown
  - skill execution audit
- migration test from empty Postgres
- migration test from existing SQLite fixture

## 12. Production Recommendation

Use managed Postgres first, not self-hosted, unless there is a strict infra
reason.

Good options:

- Supabase Postgres
- Neon
- Railway Postgres for early production
- AWS RDS / Cloud SQL for mature production

For this project, a pragmatic path is:

```text
Local dev: SQLite or local Postgres
Staging: managed Postgres
Production: managed Postgres + object storage + optional Redis
```

## 13. Non-Goals

- Do not introduce MongoDB for core relational tenant data.
- Do not keep production uploads only on local disk.
- Do not split services before the database boundary is stable.
- Do not migrate every query by hand without tests.
- Do not allow mixed org data during migration.
