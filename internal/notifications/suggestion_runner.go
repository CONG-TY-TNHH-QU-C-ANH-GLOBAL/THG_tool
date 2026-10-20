package notifications

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
)

// SuggestionRunner moves optional notification enrichment off lead ingestion
// while bounding both concurrent calls and their duration. Try returns false
// when saturated so the caller can immediately deliver the base lead notice.
type SuggestionRunner struct {
	slots         chan struct{}
	timeout       time.Duration
	timeoutForOrg func(int64) time.Duration
}

func NewSuggestionRunner(maxConcurrent int, timeout time.Duration, policies ...func(int64) time.Duration) *SuggestionRunner {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	runner := &SuggestionRunner{slots: make(chan struct{}, maxConcurrent), timeout: timeout}
	if len(policies) > 0 {
		runner.timeoutForOrg = policies[0]
	}
	return runner
}

func (r *SuggestionRunner) Try(build func(context.Context) models.LeadSuggestion, deliver func(models.LeadSuggestion)) bool {
	return r.TryWithFallback(build, deliver, models.LeadSuggestion{})
}

// TryWithFallback delivers an honest operator draft when optional enrichment
// times out or panics. Saturation remains the caller's responsibility.
func (r *SuggestionRunner) TryWithFallback(build func(context.Context) models.LeadSuggestion, deliver func(models.LeadSuggestion), unavailable models.LeadSuggestion) bool {
	return r.TryForOrgWithFallback(0, build, deliver, unavailable)
}

// Org-specific time budgets share the same slots; a longer analysis budget
// cannot create another pool or increase the concurrency ceiling.
func (r *SuggestionRunner) TryForOrgWithFallback(orgID int64, build func(context.Context) models.LeadSuggestion, deliver func(models.LeadSuggestion), unavailable models.LeadSuggestion) bool {
	if r == nil || build == nil || deliver == nil {
		return false
	}
	timeout := r.timeout
	if r.timeoutForOrg != nil {
		if selected := r.timeoutForOrg(orgID); selected > 0 {
			timeout = selected
		}
	}
	select {
	case r.slots <- struct{}{}:
		go func() {
			defer func() { <-r.slots }()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			result := make(chan models.LeadSuggestion, 1)
			go func() {
				defer func() {
					if recover() != nil {
						result <- unavailable
					}
				}()
				result <- build(ctx)
			}()
			select {
			case suggestion := <-result:
				deliver(suggestion)
			case <-ctx.Done():
				deliver(unavailable)
				<-result // keep context-ignoring provider work bounded by the slot
			}
		}()
		return true
	default:
		return false
	}
}
