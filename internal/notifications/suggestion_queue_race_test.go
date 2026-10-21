package notifications

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestConcurrentQueueStartAndExpiryDeliverEachJobOnce(t *testing.T) {
	for round := 0; round < 20; round++ {
		queueExpiryRound(t)
	}
}

func queueExpiryRound(t *testing.T) {
	t.Helper()
	r := NewQueuedSuggestionRunner(2, time.Second, func(int64) bool { return true })
	r.queueWait = 10 * time.Millisecond
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var active atomic.Int32
	done := make(chan int, 20)
	for i := 0; i < 10; i++ {
		i := i
		if !r.Try(func(context.Context) models.LeadSuggestion {
			if active.Add(1) > 2 {
				t.Error("queue exceeded the two active provider slots")
			}
			defer active.Add(-1)
			<-release
			return models.LeadSuggestion{}
		}, func(models.LeadSuggestion) { done <- i }) {
			t.Fatal("burst within queue capacity was rejected")
		}
	}
	time.AfterFunc(r.queueWait, func() { once.Do(func() { close(release) }) })
	seen := map[int]bool{}
	for i := 0; i < 10; i++ {
		select {
		case job := <-done:
			if seen[job] {
				t.Fatalf("job %d was delivered twice during expiry/start race", job)
			}
			seen[job] = true
		case <-time.After(time.Second):
			t.Fatal("job was lost during expiry/start race")
		}
	}
}
