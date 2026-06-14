// Package ports holds the CONSUMER-OWNED interfaces of the Facebook service
// (PORTS_AND_ADAPTERS.md §2-3). A Facebook workflow that needs to QUEUE an outbound
// action depends on this narrow planner port — NOT on outbound's internals — so the
// dependency points at a small, stable contract the service itself owns.
//
// SCAFFOLD (Phase D): defines the boundary; production is NOT yet wired to it.
// Today cmd/scraper's queueLeadOutreach is called directly; this port is its
// intended typed seam (roadmap Phase C/D). Minimal by design — no premature value
// structs cross the boundary.
package ports

import "context"

// OutboundPlanner queues an outbound action through the shared outbound spine
// (readiness/coverage/quality/dedup/policy gates). The Facebook service OWNS this
// interface because it CONSUMES the planner; the outbound module provides the
// adapter, injected at the composition root. Facebook workflows depend on this, not
// on outbound internals — keeping outbound vertical-neutral.
type OutboundPlanner interface {
	// QueueComment plans a comment on an existing lead, scoped to one org/account/
	// user. Returns the shared status summary (queued / no-ready-account / coverage /
	// blocked). Tenant-scoped to orgID.
	QueueComment(ctx context.Context, orgID, leadID, userID, accountID int64) (summary string, err error)
}
