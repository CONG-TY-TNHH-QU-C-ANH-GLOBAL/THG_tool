package facebook

import (
	"context"
	"time"
)

// Analysis can use five seconds, then sourcing needs nine seconds to start.
// Reserve a complete pipeline budget only for explicitly enabled analysis orgs.
func LeadSuggestionTimeout(base time.Duration, enabled bool, scope OrgAllowlist, orgID int64) time.Duration {
	if enabled && scope.Allows(orgID) && base < 24*time.Second {
		return 24 * time.Second
	}
	return base
}

// Optional wording must return before the runner deadline, preserving an offer
// already retrieved instead of replacing it with a generic timeout notice.
func leadWordingContext(parent context.Context) (context.Context, context.CancelFunc, bool) {
	budget := 4 * time.Second
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline) - 400*time.Millisecond
		if remaining < budget {
			budget = remaining
		}
	}
	if budget <= 0 || parent.Err() != nil {
		return parent, func() {}, false
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	return ctx, cancel, true
}
