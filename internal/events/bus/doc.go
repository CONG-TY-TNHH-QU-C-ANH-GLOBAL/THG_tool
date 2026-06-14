// Package bus is the (future) home of the in-process subscriber fan-out that the
// relay delivers to — the durable-aware successor of the current in-memory SSE bus.
//
// SCAFFOLD (Phase E): boundary marker only. The CURRENT in-memory SSE bus still
// lives at internal/events/bus.go (package events) and remains the working SSE
// path; it is NOT moved or changed by this commit. When the outbox + relay land
// (TRANSACTIONAL_OUTBOX.md), that bus becomes a SUBSCRIBER of the relay rather than
// an event store, and consolidates here.
//
//   - Allowed imports (conceptual): events/outbox (envelope), stdlib.
//   - Forbidden imports (conceptual): store, services/*, drivers/*. The bus moves
//     data to subscribers; it owns no business logic and is not a source of truth.
package bus
