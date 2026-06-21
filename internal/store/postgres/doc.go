// Package postgres is the PostgreSQL foundation for the outbound task
// lifecycle (PR11). It provides OutboundStore, a pgx/pgxpool-backed adapter
// that implements the SAME outbound lifecycle method set as the active
// SQLite store — the PR10 seam internal/server/agent.OutboundLifecycleRepository
// (list planned -> claim -> finalize -> reset-stale).
//
// Scope and non-goals:
//
//   - This package is NOT wired into application startup. SQLite
//     (internal/store) remains the active runtime implementation; there is no
//     runtime DB selection here (see the PR9 data-platform ADR).
//   - The adapter reuses existing domain types (models.OutboundMessage,
//     models.ExecutionState, models.VerificationOutcome, outbound.ClaimResult).
//     It introduces NO new DTO/domain layer and NO mapper layer — only small
//     private scan helpers that map PostgreSQL-strict values into those types.
//   - The authoritative state machine (the row-level CAS on outbound_messages)
//     is preserved exactly, and the OutboundClaimed telemetry event is emitted
//     on claim (PR12 parity with the SQLite path).
//   - The best-effort execution_attempts transition-audit append is
//     deliberately NOT performed by this adapter. execution_attempts is owned
//     exclusively by the coordination domain (enforced by
//     scripts/check_topology.sh §5) and is wired into the SQLite path through a
//     composition-root Hooks.RecordTransition closure — it is NOT owned by the
//     outbound storage layer itself (the outbound subpackage also only calls
//     the hook, never INSERTs the table). A PostgreSQL coordination transition
//     writer that shares the claim/finalize transaction is a dedicated
//     follow-up that extends the append-only-ledger domain to pgx; it is out of
//     this adapter's scope. action_ledger is likewise coordination-owned and is
//     a Queue-path side effect, outside the outbound lifecycle seam entirely.
//
// The schema lives in migrations/001_outbound_core.sql and is applied
// explicitly (tests / future operator cutover), never by the runtime migrator.
package postgres
