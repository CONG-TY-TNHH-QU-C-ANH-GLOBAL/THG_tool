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
//     is preserved exactly. The best-effort execution_attempts audit append and
//     telemetry events the SQLite path also performs are intentionally deferred
//     to a follow-up PR (they require the execution_attempts table + the
//     coordination hook surface, which is out of this foundation's scope).
//
// The schema lives in migrations/001_outbound_core.sql and is applied
// explicitly (tests / future operator cutover), never by the runtime migrator.
package postgres
