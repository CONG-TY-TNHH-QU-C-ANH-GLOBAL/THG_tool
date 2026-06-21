package agent

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/outbound"
)

// OutboundLifecycleRepository is the consumer-owned port for the outbound
// task lifecycle the agent outbox loop depends on: read the claimable
// (planned) tasks, claim one (lease + execution_id), finalize the terminal
// CAS, and reset stale executing rows. It is the seam a future
// PostgreSQL-backed implementation must satisfy; the active runtime
// implementation today remains the SQLite-backed *store.Store (see the
// outbound subpackage and internal/store/outbound_aliases.go).
//
// PR10 introduces this boundary only — it changes no behavior, no SQL, no
// schema, and no runtime wiring. The interface mirrors the EXACT existing
// store signatures (reusing models.* and outbound.ClaimResult) so no DTO,
// mapper, or call-site rewrite is required. The field h.db stays typed as
// the concrete *store.Store; the compile-time assertion below is the only
// binding between the port and its current implementation. A later PR can
// narrow the field/constructor to this interface once a second backend
// exists.
type OutboundLifecycleRepository interface {
	// GetOutboundByExecutionStateForOrg lists tasks in a given execution
	// state (the loop reads ExecPlanned candidates before claiming).
	GetOutboundByExecutionStateForOrg(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error)

	// ClaimPlannedOutboundForOrg moves one planned task to executing under a
	// fresh execution_id + lease (row-level CAS).
	ClaimPlannedOutboundForOrg(orgID, id int64, workerID string, leaseDuration time.Duration) (*outbound.ClaimResult, error)

	// FinalizeOutboundAttempt applies the terminal-state CAS gated on the
	// execution_id token.
	FinalizeOutboundAttempt(ctx context.Context, orgID, id int64, executionID string, terminalState models.ExecutionState, verificationOutcome models.VerificationOutcome) (finalized bool, currentState models.ExecutionState, currentOutcome models.VerificationOutcome, currentExecID string, err error)

	// ResetStaleExecutingForOrg evicts expired-lease executing rows back to
	// planned (the global fallback window for legacy null-lease rows).
	ResetStaleExecutingForOrg(orgID int64, staleAfter time.Duration) error
}

// Compile-time assertion: the active SQLite-backed store satisfies the port
// with its existing methods — no adapter, no type mapping. Build fails here
// if a lifecycle signature drifts.
var _ OutboundLifecycleRepository = (*store.Store)(nil)
