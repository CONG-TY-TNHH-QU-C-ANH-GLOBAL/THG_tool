package main

import (
	"testing"
	"time"

	"github.com/thg/scraper/internal/services/facebook/crawlcampaign"
	"github.com/thg/scraper/internal/session/accountsafety"
)

// The fresh-lead campaign store is Postgres-only, so on a SQLite runtime the
// orchestrator must not be constructed at all — the campaign path fails closed
// rather than spin against a store that only returns ErrUnsupportedDialect. This
// is the default-off safety backstop even if the flag is flipped on the wrong
// runtime.
func TestFreshLeadOrchestratorNilOnSQLiteRuntime(t *testing.T) {
	db, js := newIntakeEnv(t) // SQLite temp store
	coord := accountsafety.NewCoordinator(accountsafety.DefaultConfig(), 15*time.Minute)
	if newFreshLeadCampaignOrchestrator(db, js, coord) != nil {
		t.Fatal("SQLite runtime must yield a nil orchestrator (fail-closed), got non-nil")
	}
}

// Nil dependencies must also yield nil (guards the wiring order in main).
func TestFreshLeadOrchestratorNilOnMissingDeps(t *testing.T) {
	if newFreshLeadCampaignOrchestrator(nil, nil, nil) != nil {
		t.Fatal("missing deps must yield a nil orchestrator, got non-nil")
	}
}

// The dispatched _fresh_lead_cutoff_at must preserve the run's exact fresh-cutoff
// instant, including sub-second precision, so PR-M5 freshness classification sees
// the same boundary the store computed. RFC3339 (whole seconds) would truncate
// it; RFC3339Nano round-trips exactly.
func TestFreshLeadCutoffPreservesNanosecondPrecision(t *testing.T) {
	cutoff := time.Date(2026, 7, 22, 12, 0, 0, 123456789, time.UTC)
	extras := freshLeadDispatchExtras(crawlcampaign.PooledClaim{
		Fence:         crawlcampaign.RunFence{OrgID: 1, RunID: 9, Attempt: 1},
		FreshCutoffAt: cutoff,
	})
	raw, ok := extras["_fresh_lead_cutoff_at"].(string)
	if !ok {
		t.Fatalf("_fresh_lead_cutoff_at missing or not a string: %#v", extras["_fresh_lead_cutoff_at"])
	}
	got, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatalf("parse cutoff %q: %v", raw, err)
	}
	if !got.Equal(cutoff) {
		t.Fatalf("cutoff lost precision: serialized %q parsed to %s, want %s", raw, got, cutoff)
	}
	if got.Equal(cutoff.Truncate(time.Second)) {
		t.Fatalf("cutoff was truncated to the second (%q) — nanosecond precision lost", raw)
	}
}
