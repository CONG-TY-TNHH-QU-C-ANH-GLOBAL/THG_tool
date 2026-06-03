# Migrations

This directory holds versioned schema changes that run on top of the
legacy `s.migrate()` baseline in `schema.go`.

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

`s.migrate()` in `schema.go` is the **frozen legacy SQLite baseline**. Do NOT
add schema to it and do NOT bump `schemaBootstrapVersion`. ALL new schema
changes — for both SQLite and Postgres — go in THIS directory as numbered
`.up.sql` files.

## File naming

```
NNNN_short_description[__sqlite|__postgres].up.sql
```

- `NNNN` — zero-padded monotonic version (e.g. `0002`). The current
  baseline is implicitly version 1, so all hand-written migrations
  start at 0002.
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
