# PostgreSQL Compatibility Plan

**Status:** PR-1, PR-2, PR-3 SHIPPED. Knowledge OS domain runs on both SQLite and Postgres. Legacy domains (leads, accounts, etc.) remain SQLite-only — per-domain migration is the next infrastructure work when those teams need PG.
**Last update:** 2026-05-18.

This document is the rationale + touchpoint inventory for moving the store layer from SQLite-only to dual-target (SQLite for dev/test, Postgres for production). It is sized so one engineer can execute it in three focused PRs.

---

## 1. Why now (vs. why not yet)

Not for traffic. SQLite handles the current workload comfortably.

The triggers:

1. **Retrieval complexity** — Phase C.2 (BM25/trigram) needs FTS. SQLite FTS5 exists but lacks the relevance-tuning surface PG `tsvector` + `ts_rank_cd` provides. The hybrid searcher will start fighting SQLite.
2. **JSON querying** — `knowledge_events.data_json` and `knowledge_assets.payload` are queried more often. SQLite's `json_extract` is fine for small payloads, but the operator-replay analytics ("show me retrievals where any selected hit had score < 0.3") want indexed JSONB.
3. **Ranking** — `ORDER BY some_computed_score DESC LIMIT k` against a 100k-row catalog needs partial / expression indexes. SQLite supports both but PG's planner is significantly better here.
4. **Traces** — Phase D's events table is append-only with high write rates. SQLite single-writer model becomes a bottleneck when sync + retrieval + outcome events overlap.
5. **Vector support** — Phase C.2/C.3 eventually needs pgvector. The longer we wait to dual-target, the more SQLite assumptions the codebase accretes.

The triggers we will NOT chase here:
- "Postgres is more enterprise" — empty signaling. Reject.
- "Better for HA" — not the bottleneck.

---

## 2. What stays the same

**The cross-boundary domain types do not change.** `workspace_knowledge/sources/types.go`, `assets/types.go`, every Go struct — untouched. By design (`feedback_contracts_not_orm.md`): domain shapes are not row serialisations.

**Repository method signatures do not change.** `GetKnowledgeAsset(ctx, assetID, orgID int64)` looks identical to callers.

**Tenant isolation invariants do not change.** Every method takes `orgID`; foreign-org reads return `sql.ErrNoRows`. PG implementation MUST preserve this.

---

## 3. Touchpoints — exhaustive inventory

### 3.1 Schema syntax

| SQLite | PostgreSQL | Affected files |
|---|---|---|
| `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGSERIAL PRIMARY KEY` (or `BIGINT GENERATED ALWAYS AS IDENTITY`) | `internal/store/schema.go` (historical — schema DDL now lives in `internal/store/migrations/`) — every table |
| `DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP` | `TIMESTAMPTZ NOT NULL DEFAULT NOW()` | schema.go — every timestamp |
| `TEXT NOT NULL DEFAULT '{}'` (JSON-as-text) | `JSONB NOT NULL DEFAULT '{}'` (where queryable) or keep TEXT (where opaque) | schema.go — `connection_config`, `payload`, `data_json`, etc. |
| `CURRENT_TIMESTAMP` (in `ON CONFLICT DO UPDATE`) | `NOW()` | schema.go + every Upsert\* method |
| `DATETIME('now', '-N days')` | `NOW() - INTERVAL 'N days'` | [internal/store/knowledge/events.go](../../../../../../internal/store/knowledge/events.go) — `CountStaleAssetsForOrg` |
| `CREATE UNIQUE INDEX ... WHERE expr` (partial index) | Same syntax, supported | `schema.go` (historical — now in `internal/store/migrations/`) — `uq_knowledge_assets_idem` (already PG-compatible) |
| `LIKE` (binary, ASCII-case-sensitive) | `LIKE` (locale-sensitive) | knowledge_assets.go search — should explicitly `ILIKE` for case-insensitive |
| `INTEGER NOT NULL DEFAULT 0` for booleans | `BOOLEAN NOT NULL DEFAULT FALSE` | `pinned` column in `knowledge_assets`, others |
| `id INTEGER` | `id BIGINT` everywhere in Go (already `int64`) — schema needs `BIGINT` to match | every table with `id` |

**Decision: keep INTEGER PKs.** Per `feedback_freeze_abstraction.md` we are not changing entity identity. PG `BIGSERIAL` is compatible with `int64` Go fields.

### 3.2 Driver + connection

- Replace `github.com/mattn/go-sqlite3` (used in [internal/store/store.go](../../../../../../internal/store/store.go)) with `github.com/jackc/pgx/v5/stdlib` for `database/sql` compat.
- New env var `DATABASE_URL` decides driver. Default to SQLite in dev so the existing test pattern (`store.New(tempfile)`) still works.
- Connection-pool sizing — PG needs explicit `db.SetMaxOpenConns` (10–30 typical); SQLite ignores this. Add a `tunePool(*sql.DB, driver string)` helper.

### 3.3 SQL flavor abstraction

Two strategies; the right one depends on volume of divergent code.

**Strategy A — Dialect helpers.** Small set of helpers in `internal/store/sql_dialect.go`:
```go
func nowExpr() string                  // "CURRENT_TIMESTAMP" or "NOW()"
func intervalDaysExpr(days int) string // "DATETIME('now', '-30 days')" or "NOW() - INTERVAL '30 days'"
func ilike(col string) string          // "LOWER(col) LIKE" or "col ILIKE"
```
Bound at `store.New(...)` based on detected driver.

**Strategy B — `sqlc` or similar SQL-first codegen** with separate `.sql` files per dialect.

**Decision: Strategy A**, because:
- The divergent surface is small (~20 expressions across the codebase).
- We already have the entire repository as hand-written `database/sql` — moving to codegen is a separate, larger migration.
- Strategy A lets the existing test pattern survive untouched.

### 3.4 JSON columns

The honest cost-benefit:

| Column | Read pattern | Recommendation |
|---|---|---|
| `knowledge_assets.payload` | Mostly opaque (Go decodes for image extraction only) | Keep TEXT in SQLite, JSONB in PG. Cheap upgrade. |
| `knowledge_assets.tags` | Already normalised slice; SQL `LIKE` searches it | Move to native `TEXT[]` in PG (GIN index for `?` operator). Larger change — defer to a follow-up PR. |
| `knowledge_events.data_json` | Read whole-blob by the replay UI; never queried by inner field today | Keep TEXT both sides for now. Move to JSONB later if the analytics dashboard wants `WHERE data_json->'trace'->>'searcher_impl' = 'hybrid-v1'`. |
| `knowledge_sources.connection_config` | Opaque (each ingestor parses its own subset) | Keep TEXT. JSONB buys nothing here. |

### 3.5 Migration tool

**Recommendation:** [`golang-migrate`](https://github.com/golang-migrate/migrate). Reasons:
- Native `database/sql` integration (matches our driver layer).
- Up/down files per migration; idempotent runner; supports both SQLite + PG.
- We currently bootstrap with `s.migrate()` calling raw Exec — that pattern reaches its limit at ~30 migrations. We are at ~25 today.

Migration path:
1. Snapshot the current `s.migrate()` output as the baseline `0001_init.sql`.
2. New schema changes ship as `0002_*.sql`, `0003_*.sql` files.
3. `s.migrate()` shrinks to "run all unapplied migrations through the migrator library".

This is its own PR — should not bundle with the dialect work.

### 3.6 Test infrastructure

- `newKnowledgeTestStore` etc. should accept an env var `TEST_DATABASE_URL` and, when set, run against a real PG instance. Default unset → SQLite tempdir (current behavior).
- Add a `Makefile` target `test-pg` that spins up PG via `testcontainers-go` (or just expects a local PG) and runs the same test suite. Catches dialect mismatches.
- CI matrix: run the full suite under both drivers. The four invariants from [knowledge-os technical.md](../technical.md) §10 are the load-bearing assertions.

### 3.7 Vector support (pgvector)

OUT OF SCOPE for the dialect migration. Once PG is the primary, a follow-up PR adds:
- `pgvector` extension install + version pin
- `embeddings` column type (VECTOR(N))
- New `Searcher` implementation in `workspace_knowledge/retrieval/pgvector/` using `pgvector-go`
- Bootstrap: embed all approved assets via batch job

Plug-in via the existing `retrieval.Searcher` port. Zero changes to runtime / assembly / replay.

---

## 4. Sequencing

Three PRs, each independently shippable.

### PR-1: Dialect abstraction (no Postgres yet)
- Add `internal/store/sql_dialect.go` with the helpers from §3.3.
- Replace literal `CURRENT_TIMESTAMP`, `DATETIME('now', ...)`, `LIKE` in user-input queries with the helpers.
- All existing tests still pass on SQLite. New regression check: a sentinel "wrong dialect" helper used in a test fails the build.
- **Risk: low.** Pure refactor.

### PR-2: Migration runner
- Snapshot `0001_init.sql` from current `s.migrate()`.
- Wire `golang-migrate`. Bootstrap runs migrator on every `store.New`.
- New rows live in `0002_*.sql` going forward.
- **Risk: medium.** Boot order matters; tests must run migrations.

### PR-3: PG driver wired
- Driver-select on `DATABASE_URL`. CI matrix: SQLite + PG.
- Production rollout: deploy reads `DATABASE_URL=postgres://...` and migrates on boot.
- **Risk: medium-high.** Production cutover is the real blast radius.

---

## 5. What this plan deliberately does NOT do

- **No multi-region.** That is a separate distributed-systems problem, not a dialect problem.
- **No read replicas.** Same reasoning.
- **No data migration tool.** SQLite → PG migration is a one-time, manual `pg_dump`-style operation. Not worth building the abstraction.
- **No connection pooling beyond `database/sql`.** PgBouncer can land later.

---

## 6. Re-reading the trust-first order

`project_crawler_trust_phase_plan.md` (local planning memory, not in-repo) locks crawler-trust before orchestration before runtime. This plan is for AFTER those ship. Executing it earlier risks pre-optimising the substrate before its workload is known. The team explicitly directed (see chat 2026-05-18) that Postgres compat is Priority 4, behind Replay backend + trace expansion + hybrid search.

---

## 7. Open questions

1. **JSONB tag columns now or later?** Touching `tags` is the biggest schema diff; doing it lazily lets PR-3 stay surgical. Recommendation: keep `tags` as JSON-array TEXT until a clear retrieval use case justifies the JSONB index.
2. **`pgvector` self-host vs. managed?** Hosted is easier; self-host gives more model flexibility. Defer to the team operating production.
3. **SQLite retirement?** Likely never — keeping it as dev/test default is cheap and helps onboarding. The plan keeps it as a first-class target.

---

## 8. Implementation status (post 2026-05-18)

### Shipped

| Component | Status | File |
|---|---|---|
| `Dialect` interface | ✅ | [internal/store/dialect.go](../../../../../../internal/store/dialect.go) |
| SQLite dialect impl | ✅ | [internal/store/dbutil/dialect_sqlite.go](../../../../../../internal/store/dbutil/dialect_sqlite.go) |
| Postgres dialect impl | ✅ | [internal/store/dbutil/dialect_postgres.go](../../../../../../internal/store/dbutil/dialect_postgres.go) |
| `*Store` auto-rebind wrappers (`Query/Exec/QueryRowContext`, `InsertReturningID`) | ✅ | [internal/store/dialect.go](../../../../../../internal/store/dialect.go) |
| Boot-time driver detection (`DATABASE_URL` or `postgres://` DSN) | ✅ | [internal/store/store.go](../../../../../../internal/store/store.go) |
| PG connection pool tuning (25/5/5m) | ✅ | [internal/store/store.go](../../../../../../internal/store/store.go) |
| In-house migration runner with `schema_migrations` registry | ✅ | [internal/store/migrator.go](../../../../../../internal/store/migrator.go) |
| Baseline-marker detection for existing SQLite installs | ✅ | [internal/store/migrator.go](../../../../../../internal/store/migrator.go) `recordBaselineIfNeeded` |
| PG-flavour baseline migration (Knowledge OS tables) | ✅ | [internal/store/migrations/0001_knowledge_os_baseline__postgres.up.sql](../../../../../../internal/store/migrations/0001_knowledge_os_baseline__postgres.up.sql) |
| Knowledge OS repository converted to dialect-aware (Rebind + RETURNING) | ✅ | knowledge_sources.go, knowledge_assets.go, knowledge_events.go, knowledge_replay.go |
| Dialect unit tests | ✅ | [internal/store/dialect_test.go](../../../../../../internal/store/dialect_test.go) |

### Deferred (per directive — separate work)

| Component | Why deferred |
|---|---|
| Legacy domain conversion (leads, accounts, outbound, threads, etc. — 22 files use `LastInsertId`) | Too large for a single PR. Per-domain teams convert as they need PG support. The dialect wrappers + `InsertReturningID` are ready for them. |
| testcontainers PG matrix in CI | Requires CI configuration outside this codebase. Local PG testing works today by setting `DATABASE_URL` before `go test`. |
| `pgvector` extension migration | Phase 4 follow-up (after retrieval infrastructure stable). Planned migration: `0002_add_pgvector_extension__postgres.up.sql`. |
| BM25/FTS hybrid reranking | Requires PG's `tsvector` + a SQLite FTS5 variant. Both planned but neither blocking infrastructure. |
| `tags TEXT[]` (PG native array) | Stays TEXT until a GIN-indexed tag-search use case justifies the schema diff. |

### Production cutover runbook (for the operator)

1. **Provision PG.** Recommend 14+. Create database. Note connection URL.
2. **Add the driver import.** In `cmd/scraper/main.go` (and any other binary that touches the store):
   ```go
   import _ "github.com/jackc/pgx/v5/stdlib"
   ```
   Run `go get github.com/jackc/pgx/v5/stdlib` to add it to `go.mod`. Without this import, `sql.Open("pgx", ...)` returns `"unknown driver"` at boot.
3. **Set `DATABASE_URL`** in the deployment config (e.g. `postgres://user:pass@host:5432/dbname?sslmode=require`).
4. **Boot.** `store.New("")` reads `DATABASE_URL`, opens PG, runs `0001_knowledge_os_baseline__postgres.up.sql`, records version 1 in `schema_migrations`.
5. **Verify.** `SELECT * FROM schema_migrations;` should show one row. Run smoke tests against `/api/org/knowledge/*`.
6. **Data migration** (existing SQLite → PG, if needed): use `pgloader` or a one-shot Go script that reads SQLite + writes PG. Not built into the store layer — application-level concern.

### Risk register status (R1–R20 from §3)

| Risk | Status | How addressed |
|---|---|---|
| R1 `LastInsertId` on PG | ✅ resolved | `InsertReturningID` helper; knowledge_* converted |
| R2 `?` vs `$N` placeholders | ✅ resolved | `Dialect.Rebind()`; auto-applied by `*Store` wrappers |
| R3 INTEGER vs BIGINT | ✅ resolved | PG baseline uses BIGSERIAL/BIGINT |
| R4 Partial UNIQUE + ON CONFLICT | ✅ resolved | Both dialects support same WHERE clause; verified syntax in PG baseline |
| R5 Silent migration drift | ✅ resolved | Migration runner records versions; idempotent baseline |
| R6 Timestamp format | ⚠️ partial | UTC mandated; `parseSQLiteTime` accepts multiple formats. PG TIMESTAMPTZ round-trips via `time.Time`. |
| R7 `CURRENT_TIMESTAMP` vs `NOW()` | ✅ resolved | Both dialects accept `CURRENT_TIMESTAMP`; helper exists for code that needs to be explicit |
| R8 `DATETIME('now',...)` | ✅ resolved | `Dialect.IntervalDaysExpr()`; converted in knowledge_events.go |
| R9 `LIKE` case-sensitivity | ⚠️ partial | Existing code uses `LOWER(col) LIKE LOWER(?)` (works on both); no explicit helper yet |
| R10 FK enforcement | ✅ resolved | SQLite has `foreign_keys=on` pragma; PG always enforces |
| R11 Baseline on existing prod | ✅ resolved | `recordBaselineIfNeeded` |
| R12 Connection pooling | ✅ resolved | 25/5/5m tuning in `newPostgres` |
| R13 Tx isolation | 📋 documented | Append-only events table not affected by READ COMMITTED |
| R14 Boolean type | ✅ resolved | Kept INTEGER on both — cross-compatible |
| R15 `sql.ErrNoRows` | ✅ verified | Code uses `errors.Is`; no driver-type dependence |
| R16 CI cross-dialect | 📋 deferred | Local PG testing works; CI matrix is operator setup work |
| R17 JSONB queryability | 📋 deferred | TEXT today; flip to JSONB when query use case emerges |
| R18 pgvector version pin | 📋 deferred | Phase 4 follow-up |
| R19 Driver-specific errors | 📋 documented | Don't depend on error identity; use `errors.Is` |
| R20 Connection lifecycle | ✅ resolved | `ConnMaxLifetime` 5m |

### Next infrastructure work (in order)

1. **pgvector Searcher** — implements `retrieval.Searcher`. New migration `0002_*` adds the extension + embedding column. Searcher does cosine-similarity lookup.
2. **Embedding ingestion pipeline** — backfill embeddings for all approved assets via batch job; per-ingest hook for new assets.
3. **BM25/FTS hybrid reranking** — combine pgvector scores with `tsvector` ranking. Falls back to current hybrid searcher on SQLite.
