// Package observability exposes READ-ONLY surfaces over the verified
// execution substrate (execution_attempts + account_runtime_state +
// behaviour profiles). It is Step 4a of the trust-first roadmap — the
// dashboard's view into reality, not a place for new intelligence.
//
// Hard rules for this package:
//   - No write endpoints. Every handler is GET.
//   - No auto-decisions, no scoring, no orchestration. Pure SELECT and
//     project. The future PR-5 Account Orchestrator consumes this data
//     server-side; the dashboard consumes it client-side.
//   - Org-scoped via the protected-router middleware (c.Locals("org_id")).
//   - Bounded time windows + row limits — these surfaces back human
//     observation, not bulk export.
package observability

import (
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/store"
)

// Deps captures the (read-only) store dependency. Kept as a struct so
// future observability surfaces (crawl url-repair distribution, action-
// ledger health, classifier-decision counters) can be added without
// re-threading the router.
//
// PromptIsSelfSufficient is injected so the conflict-candidate handler
// can reuse the orchestrator's self-sufficient gate without this package
// importing internal/ai. nil-tolerant: when nil, the false-negative
// conflict heuristic returns no rows (the false-positive heuristic still
// works since it has no internal/ai dependency).
type Deps struct {
	DB                    *store.Store
	PromptIsSelfSufficient func(prompt string) bool
}

// Routes registers the GET-only observability endpoints under group.
// Caller must have already applied JWT + tenant middleware.
func Routes(group fiber.Router, deps Deps) {
	exec := group.Group("/observability/execution")
	exec.Get("/distribution", executionDistribution(deps))
	exec.Get("/recent", executionRecent(deps))
	exec.Get("/account-health", executionAccountHealth(deps))

	// Watchpoint B — prompt routing observability.
	pr := group.Group("/observability/prompt-routing")
	pr.Get("/distribution", promptRoutingDistribution(deps))
	pr.Get("/recent", promptRoutingRecent(deps))
	pr.Get("/conflicts", promptRoutingConflicts(deps))
	pr.Get("/missing-signals", promptRoutingMissingSignals(deps))
}
