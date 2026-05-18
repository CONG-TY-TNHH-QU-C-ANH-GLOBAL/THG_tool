# Migrations

This directory holds versioned schema changes that run on top of the
legacy `s.migrate()` baseline in `schema.go`.

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

1. **Idempotent.** Use `IF NOT EXISTS`. Migration may re-run if the
   `INSERT INTO schema_migrations` step ever fails — your DDL must
   tolerate that.
2. **One statement per `;`.** The runner does NOT split — it hands the
   full file to `ExecContext`. PostgreSQL accepts multi-statement
   strings; SQLite (modernc) also does as of recent versions. Verify
   on both dialects before merging.
3. **No transaction wrapping.** Some PG operations cannot run inside
   a transaction (e.g. `CREATE INDEX CONCURRENTLY`). The migration is
   the author's atomicity decision, not the runner's.
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
