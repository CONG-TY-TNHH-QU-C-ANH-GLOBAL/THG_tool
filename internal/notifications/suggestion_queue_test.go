package notifications

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestQueuedSuggestionBurstKeepsFIFOAndConcurrency(t *testing.T) {
	r := NewQueuedSuggestionRunner(1, 2*time.Second, func(org int64) bool { return org == 7 })
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	done := make(chan string, 23)
	var active atomic.Int32
	for i := 0; i < 23; i++ {
		i := i
		accepted := r.TryForOrgWithFallback(7, func(context.Context) models.LeadSuggestion {
			if active.Add(1) != 1 {
				t.Error("burst exceeded the provider concurrency limit")
			}
			defer active.Add(-1)
			if i == 0 {
				close(started)
				<-release
			}
			return models.LeadSuggestion{Reply: strconv.Itoa(i)}
		}, func(s models.LeadSuggestion) { done <- s.Reply }, models.LeadSuggestion{})
		if !accepted {
			t.Fatalf("burst job %d was dropped despite queue capacity", i)
		}
		if i == 0 {
			<-started
		}
	}
	if r.TryForOrgWithFallback(8, func(context.Context) models.LeadSuggestion { return models.LeadSuggestion{} }, func(models.LeadSuggestion) {}, models.LeadSuggestion{}) {
		t.Fatal("queueing unexpectedly enabled outside the selected org")
	}
	once.Do(func() { close(release) })
	assertQueuedRepliesFIFO(t, done, 23)
}

func assertQueuedRepliesFIFO(t *testing.T, done <-chan string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case value := <-done:
			if value != strconv.Itoa(i) {
				t.Fatalf("FIFO lost at %d: got %q", i, value)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("accepted burst job was lost")
		}
	}
}

func TestQueuedSuggestionCapacityFailsFast(t *testing.T) {
	r := NewQueuedSuggestionRunner(1, time.Second, func(int64) bool { return true })
	r.queueLimit = 1
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	done := make(chan struct{}, 2)
	build := func(context.Context) models.LeadSuggestion { <-release; return models.LeadSuggestion{} }
	deliver := func(models.LeadSuggestion) { done <- struct{}{} }
	for i := 0; i < 2; i++ {
		if !r.TryWithFallback(build, deliver, models.LeadSuggestion{}) {
			t.Fatal("active and one pending job must be accepted")
		}
	}
	if r.TryWithFallback(build, deliver, models.LeadSuggestion{}) {
		t.Fatal("pending capacity was exceeded")
	}
	once.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("accepted job was lost")
		}
	}
}

func TestQueuedSuggestionReceivesBudgetWhenItStarts(t *testing.T) {
	r := NewQueuedSuggestionRunner(1, 2*time.Second, func(int64) bool { return true })
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	firstDone := make(chan struct{}, 1)
	r.Try(func(context.Context) models.LeadSuggestion { <-release; return models.LeadSuggestion{} }, func(models.LeadSuggestion) { firstDone <- struct{}{} })
	remaining := make(chan time.Duration, 1)
	done := make(chan struct{}, 1)
	if !r.Try(func(ctx context.Context) models.LeadSuggestion {
		deadline, _ := ctx.Deadline()
		remaining <- time.Until(deadline)
		return models.LeadSuggestion{}
	}, func(models.LeadSuggestion) { done <- struct{}{} }) {
		t.Fatal("second job was not queued")
	}
	time.Sleep(200 * time.Millisecond)
	once.Do(func() { close(release) })
	select {
	case budget := <-remaining:
		if budget < 1900*time.Millisecond {
			t.Fatalf("waiting consumed the active analysis budget: %v", budget)
		}
	case <-time.After(time.Second):
		t.Fatal("queued job did not start")
	}
	<-firstDone
	<-done
}
