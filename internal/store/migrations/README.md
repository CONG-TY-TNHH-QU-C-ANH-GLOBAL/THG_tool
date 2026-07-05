# Migrations

This directory is the **single source of truth** for the database schema.
There is no in-code baseline anymore: `schema.go`'s `migrate()` was retired and
its final frozen schema was dumped mechanically into
`0001_legacy_baseline__sqlite.up.sql`. Every table, index, and seed row is
created by a numbered `.up.sql` file applied by the runner (`migrator.go`).

## Production-grade guarantees (the runner — `migrator.go`)

Every migration here is applied by `runMigrations()` with these properties —
this is the path that scales to a real (1M-user) Postgres production:

- **Atomic.** The migration body AND its `schema_migrations` version record
  commit in ONE transaction, or both roll back. No half-applied state; a crash
  mid-migration leaves the DB clean and the next boot retries.
- **Fail-fast.** Any error aborts boot and is surfaced — migration failures are
  production incidents, never swallowed.
- **Run-once.** Each version is recorded and never re-applied. No re-run, no
  data clobber. (Contrast the legacy `migrate()` baseline, which historically
  re-ran its whole body on a version bump — that anti-pattern is RETIRED.)
- **Dialect-aware.** SQLite + Postgres via `__sqlite` / `__postgres` suffixes.

## FROZEN baseline

`0001_legacy_baseline__sqlite.up.sql` is the **frozen SQLite baseline** — a
mechanical dump of the retired `migrate()` schema (ALTERs folded into the
final CREATEs, no churn). Do NOT edit it. ALL new schema changes — for both
SQLite and Postgres — go in THIS directory as new numbered `.up.sql` files
starting at `0002`. (The Postgres baseline, `0001_*__postgres.up.sql`, is the
dialect-split sibling for the POSTGRES_COMPAT path.)

## Bootstrap layers (data planes)

The full boot-time schema story has TWO layers, split along the data-plane
doctrine (`docs/architecture/DATABASE_OWNERSHIP.md` §Data planes):

1. **Versioned migrations (this directory) — SaaS Platform plane.** The
   run-once source of truth for every platform table (orgs, users, leads,
   ledger, knowledge metadata, ...). The `__postgres` knowledge/pgvector
   files are the staged RAG-plane schema.
2. **Local-runtime bootstrap — Local Runtime plane.** `sessions.Migrate`
   and `app.Migrate`, run by `store.initDomains()` on every boot
   (idempotent, deterministic order), own ONLY the local runtime tables:
   `browser_sessions`, `app_tasks`/`task_leads`, `browser_identities`,
   `port_registry`, `account_rate_limits`, `circuit_breaker_state`,
   `session_audit_log`, `post_seen_cache`. `internal/jobs` bootstraps
   `scheduler_jobs` the same way (its own connection today). These tables
   are deliberately NOT in the versioned baseline.

`TestNoHiddenCreateTableBootstrap` (internal/store/bootstrap_topology_test.go)
enforces the split: a production `.go` file outside the sanctioned bootstrap
list may not contain `CREATE TABLE` — new schema goes HERE as a numbered
migration. `TestBootstrap_DoubleBootIdempotent` pins layer-2 idempotency.

## File naming

```
NNNN_short_description[__sqlite|__postgres].up.sql
```

- `NNNN` — zero-padded monotonic version (e.g. `0002`). Version `0001`
  is the frozen baseline (per dialect); all new migrations start at `0002`.
- `short_description` — operator-readable name. Used in boot logs and
  `schema_migrations.name`. Use snake_case.
- `__sqlite` / `__postgres` — optional dialect filter. Without a
  suffix, the migration runs on both dialects. With a suffix, the
  runner picks the appropriate file for the boot dialect.

## Authoring rules

1. **Idempotent where cheap.** Prefer `IF NOT EXISTS`. Migrations are
   transactional + run-once, so re-runs are not expected — but defensive
   idempotency is good hygiene.
2. **Multi-statement is fine.** The runner hands the full file to one
   `ExecContext`; modernc/sqlite and pgx both execute multiple `;`-separated
   statements. Verify on both dialects before merging.
3. **Transactional by default.** The runner wraps the body + version record in
   one transaction (atomic, fail-fast). A migration that CANNOT run in a
   transaction (e.g. Postgres `CREATE INDEX CONCURRENTLY`) opts out with
   `-- migrate:notx` on its first comment line — and then owns its own atomicity.
4. **No down migrations.** Restore from backup is the only supported
   rollback. Forward-only schema discipline catches breakage early.
5. **PG-only features need a `__postgres` variant.** Examples:
   `JSONB` queries, `pgvector` columns, `tsvector`. Pair with a
   SQLite-flavour variant only if the SQLite path still needs the
   schema — otherwise omit and the SQLite boot skips it.

## Future migrations (planned, NOT yet written)

| Version | Name | Why |
|---|---|---|
| 0002 | `add_pgvector_extension__postgres` | enable `pgvector` for embedding columns (P4 follow-up: pgvector Searcher) |
| 0003 | `add_asset_embedding_column__postgres` | `embedding VECTOR(1536)` column on `knowledge_assets` |
| 0004 | `add_fts_index_for_knowledge_assets__postgres` | `tsvector` GIN index over title+description |

## See also

- [specs/POSTGRES_COMPAT_PLAN.md](../../../specs/POSTGRES_COMPAT_PLAN.md) — production-risk inventory and rollout sequence.
- [migrator.go](../migrator.go) — runner implementation.
