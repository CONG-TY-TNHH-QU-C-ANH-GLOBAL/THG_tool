// Package relay is the (future) durable event relay: it claims `pending` rows from
// the outbox table (CAS), delivers each to in-process subscribers / SSE, and marks
// them published — or backs off and eventually dead-letters
// (docs/architecture/TRANSACTIONAL_OUTBOX.md §5).
//
// SCAFFOLD (Phase E): boundary marker only — NO relay loop is implemented in this
// commit (it depends on the outbox table, which is a gated additive migration).
//
// Design invariant to preserve when implemented: in-memory Go channels may deliver
// events only AFTER the durable outbox row exists; a dropped channel send is
// recovered on the next relay pass because the row stays `pending` until acked.
// Channels are a delivery optimization, never the source of truth.
//
//   - Allowed imports (conceptual): events/outbox (envelope), store (outbox table), stdlib.
//   - Forbidden imports (conceptual): services/*, drivers/*, business domain internals.
package relay
