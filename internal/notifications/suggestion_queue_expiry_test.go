package notifications

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestQueuedExpiryKeepsHungProviderBoundedAndExplainsMissingDraft(t *testing.T) {
	r := NewQueuedSuggestionRunner(1, time.Second, func(int64) bool { return true })
	r.queueLimit, r.queueWait = 1, 20*time.Millisecond
	release, fallbackRelease := make(chan struct{}), make(chan struct{})
	var releaseOnce, fallbackOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		fallbackOnce.Do(func() { close(fallbackRelease) })
	})
	firstDone := make(chan struct{}, 1)
	r.Try(func(context.Context) models.LeadSuggestion { <-release; return models.LeadSuggestion{} }, func(models.LeadSuggestion) { firstDone <- struct{}{} })
	var calls atomic.Int32
	fallback, fallbackDone := make(chan models.LeadSuggestion, 1), make(chan struct{}, 1)
	if !r.Try(func(context.Context) models.LeadSuggestion {
		calls.Add(1)
		return models.LeadSuggestion{}
	}, func(s models.LeadSuggestion) {
		fallback <- s
		<-fallbackRelease
		fallbackDone <- struct{}{}
	}) {
		t.Fatal("pending job was not accepted")
	}
	select {
	case got := <-fallback:
		if got.ReplyStatus != "queue_expired" || got.ReplyNote == "" || got.Reply != "" {
			t.Fatalf("expiry must explain missing analysis without inventing a reply: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("queue expiry did not deliver a fallback")
	}
	if r.Try(func(context.Context) models.LeadSuggestion { return models.LeadSuggestion{} }, func(models.LeadSuggestion) {}) {
		t.Fatal("hung fallback delivery must still count toward pending capacity")
	}
	fallbackOnce.Do(func() { close(fallbackRelease) })
	releaseOnce.Do(func() { close(release) })
	<-firstDone
	<-fallbackDone
	if calls.Load() != 0 {
		t.Fatal("expired job made a late provider call")
	}
}
