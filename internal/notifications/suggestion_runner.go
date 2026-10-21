package notifications

import (
	"context"
	"sync"
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
	mu            sync.Mutex
	queueForOrg   func(int64) bool
	pending       []*suggestionJob
	queueUsed     int // Includes expired jobs until their fallback delivery returns.
	queueLimit    int
	queueWait     time.Duration
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
	return r.submit(orgID, &suggestionJob{build: build, deliver: deliver, fallback: unavailable, timeout: timeout})
}

func (r *SuggestionRunner) run(job *suggestionJob) {
	go func() {
		defer r.finished()
		if !job.expires.IsZero() && !time.Now().Before(job.expires) {
			job.deliver(job.expiredFallback())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), job.timeout)
		defer cancel()
		result := make(chan models.LeadSuggestion, 1)
		go func() {
			defer func() {
				if recover() != nil {
					result <- job.fallback
				}
			}()
			result <- job.build(ctx)
		}()
		select {
		case suggestion := <-result:
			job.deliver(suggestion)
		case <-ctx.Done():
			job.deliver(job.fallback)
			<-result // keep context-ignoring provider work bounded by the slot
		}
	}()
}
