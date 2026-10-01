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
	slots   chan struct{}
	timeout time.Duration
}

func NewSuggestionRunner(maxConcurrent int, timeout time.Duration) *SuggestionRunner {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SuggestionRunner{slots: make(chan struct{}, maxConcurrent), timeout: timeout}
}

func (r *SuggestionRunner) Try(build func(context.Context) models.LeadSuggestion, deliver func(models.LeadSuggestion)) bool {
	return r.TryWithFallback(build, deliver, models.LeadSuggestion{})
}

// TryWithFallback delivers an honest operator draft when optional enrichment
// times out or panics. Saturation remains the caller's responsibility.
func (r *SuggestionRunner) TryWithFallback(build func(context.Context) models.LeadSuggestion, deliver func(models.LeadSuggestion), unavailable models.LeadSuggestion) bool {
	if r == nil || build == nil || deliver == nil {
		return false
	}
	select {
	case r.slots <- struct{}{}:
		go func() {
			defer func() { <-r.slots }()
			ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
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
