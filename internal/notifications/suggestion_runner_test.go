package notifications

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestSuggestionRunnerSaturationFailsFast(t *testing.T) {
	runner := NewSuggestionRunner(1, time.Second)
	started, release := make(chan struct{}), make(chan struct{})
	delivered := make(chan struct{}, 1)
	if !runner.Try(func(context.Context) models.LeadSuggestion {
		close(started)
		<-release
		return models.LeadSuggestion{}
	}, func(models.LeadSuggestion) { delivered <- struct{}{} }) {
		t.Fatal("first job should be accepted")
	}
	<-started
	if runner.Try(func(context.Context) models.LeadSuggestion { return models.LeadSuggestion{} }, func(models.LeadSuggestion) {}) {
		t.Fatal("second job should be rejected while the only slot is occupied")
	}
	close(release)
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("accepted job was not delivered")
	}
}

func TestSuggestionRunnerTimeoutKeepsHungWorkBounded(t *testing.T) {
	runner := NewSuggestionRunner(1, 20*time.Millisecond)
	delivered := make(chan models.LeadSuggestion, 1)
	release := make(chan struct{})
	if !runner.Try(func(context.Context) models.LeadSuggestion {
		<-release
		return models.LeadSuggestion{Reply: "late"}
	}, func(s models.LeadSuggestion) { delivered <- s }) {
		t.Fatal("job should be accepted")
	}
	select {
	case got := <-delivered:
		if got != (models.LeadSuggestion{}) {
			t.Fatalf("timeout must degrade to the base notice, got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not deliver the base notice at timeout")
	}
	if runner.Try(func(context.Context) models.LeadSuggestion { return models.LeadSuggestion{} }, func(models.LeadSuggestion) {}) {
		t.Fatal("timed-out context-ignoring work must keep its slot occupied")
	}
	close(release)
}

func TestSuggestionRunnerPanicDegradesToBase(t *testing.T) {
	runner := NewSuggestionRunner(1, time.Second)
	delivered := make(chan models.LeadSuggestion, 1)
	if !runner.Try(func(context.Context) models.LeadSuggestion { panic("provider bug") }, func(s models.LeadSuggestion) { delivered <- s }) {
		t.Fatal("job should be accepted")
	}
	select {
	case got := <-delivered:
		if got != (models.LeadSuggestion{}) {
			t.Fatalf("panic must degrade to base notice, got %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("panic did not produce a base notice")
	}
}

func TestSuggestionRunnerTimeoutUsesOperatorFallback(t *testing.T) {
	runner := NewSuggestionRunner(1, 20*time.Millisecond)
	fallback := models.LeadSuggestion{Reply: "Please share a product link", SourcingNote: "gợi ý chưa xử lý kịp"}
	delivered := make(chan models.LeadSuggestion, 1)
	release := make(chan struct{})
	if !runner.TryWithFallback(func(context.Context) models.LeadSuggestion {
		<-release
		return models.LeadSuggestion{Reply: "late offer"}
	}, func(s models.LeadSuggestion) { delivered <- s }, fallback) {
		t.Fatal("job should be accepted")
	}
	select {
	case got := <-delivered:
		if got != fallback {
			t.Fatalf("timeout should deliver the safe fallback: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fallback not delivered")
	}
	close(release)
}
