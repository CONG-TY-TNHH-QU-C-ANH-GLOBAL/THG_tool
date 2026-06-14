// Package events is the cross-module event substrate. Today it is an in-process
// pub/sub bus for SSE (bus.go). Its TARGET role is the durable transactional outbox
// + relay: the single source of truth for critical cross-module events
// (FacebookLeadCreated, FacebookPostImported, CommentActionPosted, …).
//
// Architecture role: EVENTS / OUTBOX (domain) — see
// docs/architecture/TRANSACTIONAL_OUTBOX.md and MODULE_BOUNDARIES.md (events).
//
//   - Allowed imports (conceptual): models, store (outbox table), stdlib.
//   - Forbidden imports (conceptual): any service or driver, business domain
//     internals. Events carry data, not behavior — "when X then Y" is a process
//     manager in the owning service, not here.
//
// RULE: in-memory Go channels (this bus) may be used only AFTER a durable DB row
// exists; they must NOT be the source of truth for critical events.
//
// SCAFFOLD/STATUS (Phase A): the durable outbox table + relay do NOT exist yet
// (roadmap Phase E is the keystone). bus.go remains the in-memory SSE bus until
// each critical event is migrated onto the outbox.
package events
