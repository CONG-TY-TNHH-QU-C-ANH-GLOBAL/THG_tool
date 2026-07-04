package agent

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/outbound"
)

// OutboundLifecycleRepository is the consumer-owned port for the outbound
// task lifecycle the agent outbox loop depends on: read the claimable
// (planned) tasks, claim one (lease + execution_id), finalize the terminal
// CAS, and reset stale executing rows. It is the seam a future
// PostgreSQL-backed implementation must satisfy; the active runtime
// implementation today is the SQLite-backed [outbound.Store], reached via
// store.Store.Outbound().
//
// The method set mirrors the EXACT [outbound.Store] signatures (the
// wrapper spelling on the legacy root *Store was retired once its last
// caller migrated), so no DTO, mapper, or adapter is required.
// internal/store/postgres/outbound.OutboundStore implements the same
// method set for the future Postgres backend; the parity test suite in
// that package asserts both implementations honour the same contract.
type OutboundLifecycleRepository interface {
	// ListByState lists tasks in a given execution state (the loop reads
	// ExecPlanned candidates before claiming).
	ListByState(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error)

	// Claim moves one planned task to executing under a fresh
	// execution_id + lease (row-level CAS).
	Claim(orgID, id int64, workerID string, leaseDuration time.Duration) (*outbound.ClaimResult, error)

	// Finalize applies the terminal-state CAS gated on the execution_id
	// token.
	Finalize(ctx context.Context, orgID, id int64, executionID string, terminalState models.ExecutionState, verificationOutcome models.VerificationOutcome) (finalized bool, currentState models.ExecutionState, currentOutcome models.VerificationOutcome, currentExecID string, err error)

	// ResetStaleExecuting evicts expired-lease executing rows back to
	// planned (the global fallback window for legacy null-lease rows).
	ResetStaleExecuting(orgID int64, staleAfter time.Duration) error
}

// Compile-time assertion: the active SQLite-backed outbound store satisfies
// the port with its existing methods — no adapter, no type mapping. Build
// fails here if a lifecycle signature drifts.
var _ OutboundLifecycleRepository = (*outbound.Store)(nil)
