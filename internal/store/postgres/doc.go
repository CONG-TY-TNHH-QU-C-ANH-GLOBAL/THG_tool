// Package postgres holds the shared PostgreSQL infrastructure for the store
// layer: the pgx/pgxpool connection helper (Open) and the authoritative,
// ordered migration files under migrations/.
//
// It is deliberately thin. Per-domain PostgreSQL adapters live in their own
// subpackages so each domain owns its schema, queries, and tests:
//
//   - internal/store/postgres/outbound — the outbound task lifecycle adapter
//     (claim / finalize / reset / read), satisfying the PR10 seam
//     internal/server/agent.OutboundLifecycleRepository.
//
// Future domain adapters (coordination, action_ledger, knowledge, …) will be
// added as sibling subpackages. Migrations stay centralized here under
// migrations/ so their apply order remains deterministic — they are NOT split
// per-module until a migration runner/manifest controls ordering.
//
// Scope and non-goals (whole package tree): this code is NOT wired into
// application startup. SQLite (internal/store) remains the active runtime
// implementation; there is no runtime DB selection here (see the PR9
// data-platform ADR). The migration SQL is applied explicitly by tests / a
// future operator cutover, never by the runtime migrator.
package postgres
