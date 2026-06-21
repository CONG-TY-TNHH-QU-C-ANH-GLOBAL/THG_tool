# PostgreSQL store (shared infra + per-domain adapters)

PostgreSQL foundation for the store layer. **Foundation only — not wired into
runtime.** SQLite (`internal/store`) remains the active database; see the PR9
data-platform ADR (`docs/architecture/ADR-PR9-DATA-PLATFORM.md`) and
`specs/POSTGRES_COMPAT_PLAN.md`.

## Structure

- `pool.go` — `Open(ctx, dsn)` shared pgx/pgxpool helper (package `postgres`),
  for tests / operator tooling.
- `migrations/` — the authoritative, ordered migration files (kept centralized
  so apply order stays deterministic; not split per-module). Currently
  `001_outbound_core.sql` — PostgreSQL DDL for `outbound_messages`. Applied
  explicitly by tests / a future operator cutover, **not** by the in-house
  runtime migrator (which embeds only `internal/store/migrations`).
- `outbound/` — package `outbound`: `OutboundStore`, a pgx/pgxpool adapter
  implementing the PR10 seam `internal/server/agent.OutboundLifecycleRepository`
  (`GetOutboundByExecutionStateForOrg`, `ClaimPlannedOutboundForOrg`,
  `FinalizeOutboundAttempt`, `ResetStaleExecutingForOrg`) plus the SQLite/Postgres
  parity tests. Future domain adapters land as sibling subpackages.

## Running the integration tests locally

The integration tests are **skipped** unless `POSTGRES_TEST_DSN` is set, so
`go test ./...` stays green without a database. To run them against the PR9 dev
Postgres:

```bash
docker compose -f deploy/dev/docker-compose.yml up -d postgres
export POSTGRES_TEST_DSN='postgres://thg:thg_local_dev@localhost:5432/thg_autoflow?sslmode=disable'
go test ./internal/store/postgres/...
docker compose -f deploy/dev/docker-compose.yml down
```

Each test drops and re-applies `migrations/001_outbound_core.sql` for isolation.
When `POSTGRES_TEST_DSN` is unset, strict PostgreSQL type-scan compatibility is
**not** proven by that run.
