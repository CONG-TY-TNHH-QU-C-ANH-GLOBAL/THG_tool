package main

import (
	"testing"
	"time"

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
	if orch := newFreshLeadCampaignOrchestrator(db, js, coord); orch != nil {
		t.Fatal("SQLite runtime must yield a nil orchestrator (fail-closed), got non-nil")
	}
}

// Nil dependencies must also yield nil (guards the wiring order in main).
func TestFreshLeadOrchestratorNilOnMissingDeps(t *testing.T) {
	if orch := newFreshLeadCampaignOrchestrator(nil, nil, nil); orch != nil {
		t.Fatal("missing deps must yield a nil orchestrator, got non-nil")
	}
}
