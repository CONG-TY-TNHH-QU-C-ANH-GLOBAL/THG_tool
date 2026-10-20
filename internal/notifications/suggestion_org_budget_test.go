package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestOrgTimeoutPolicySharesConcurrencySlots(t *testing.T) {
	runner := NewSuggestionRunner(1, time.Second, func(orgID int64) time.Duration {
		if orgID == 7 {
			return 2 * time.Second
		}
		return time.Second
	})
	started := make(chan time.Duration, 1)
	release := make(chan struct{})
	delivered := make(chan struct{}, 1)
	defer func() {
		close(release)
		select {
		case <-delivered:
		case <-time.After(time.Second):
			t.Error("accepted job not delivered")
		}
	}()
	if !runner.TryForOrgWithFallback(7, func(ctx context.Context) models.LeadSuggestion {
		deadline, _ := ctx.Deadline()
		started <- time.Until(deadline)
		<-release
		return models.LeadSuggestion{}
	}, func(models.LeadSuggestion) { delivered <- struct{}{} }, models.LeadSuggestion{}) {
		t.Fatal("target org was rejected")
	}
	if remaining := <-started; remaining < 1500*time.Millisecond {
		t.Fatalf("target org did not receive its analysis budget: %v", remaining)
	}
	if runner.TryForOrgWithFallback(8, func(context.Context) models.LeadSuggestion { return models.LeadSuggestion{} }, func(models.LeadSuggestion) {}, models.LeadSuggestion{}) {
		t.Fatal("org budgets created independent slots and exceeded concurrency")
	}
}
