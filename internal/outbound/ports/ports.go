// Package ports holds the CONSUMER-OWNED interfaces of the outbound module
// (PORTS_AND_ADAPTERS.md §2). outbound coordinates actions but does not know how a
// specific vertical (Facebook, Taobao) executes them — so it OWNS the executor
// port and a vertical implements it. outbound therefore never imports a service.
//
// SCAFFOLD (Phase D): these interfaces define the boundary; production is NOT yet
// wired to them. The legacy path is the untyped Agent.ActionHandler
// (func(string, map[string]any)); these typed ports are its intended replacement
// (roadmap Phase D wiring). Kept minimal (ids + summary) on purpose — the concrete
// PlannedAction/Outcome value types are defined when the port is wired, so this
// scaffold exposes no premature, unstable cross-boundary structs.
package ports

import "context"

// ActionExecutor performs a single planned outbound action (already queued + claimed
// by the outbound spine) and reports a human-readable outcome. outbound OWNS this
// interface because it CONSUMES executors; services/facebook (and future
// services/taobao) provide an adapter that satisfies it. The composition root
// (cmd/scraper/main.go) injects the adapter — outbound never imports the service.
type ActionExecutor interface {
	// Execute runs the planned outbound row outboundID within org orgID and returns
	// a status summary. Implementations MUST be tenant-scoped to orgID and MUST
	// return human_required (not an error) on a login/checkpoint wall.
	Execute(ctx context.Context, orgID, outboundID int64) (summary string, err error)
}
