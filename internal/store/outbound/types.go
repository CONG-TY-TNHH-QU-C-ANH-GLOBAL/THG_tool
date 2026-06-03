package outbound

import (
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/models"
)

// QueueResult carries the queue-level outcome back to the caller
// alongside the new row ID. The legacy top-level store package exports
// this as `store.OutboundQueueResult` via a type alias for call-site
// back-compat — see internal/store/outbound_aliases.go.
type QueueResult struct {
	ID             int64
	ExecutionState models.ExecutionState
	Decision       GuardDecision
}

// GuardDecision is the queue-level safety check result for automated
// comments/inbox messages. AI can propose actions, but this guard is
// the final production gate before anything reaches an executable
// outbox state. Returned by [Store.CheckDedup] and
// [Store.PreflightCheck].
type GuardDecision struct {
	Allowed        bool
	Reason         string
	ExistingID     int64
	LastOutboundAt time.Time
	LastInboundAt  time.Time
	// RiskScore and RiskCeiling are populated ONLY when
	// Reason == "risk_ceiling_exceeded" (propagated from the coordination
	// CapsDecision via the BehaviourCheck hook adapter). Surfaced to the
	// operator-facing telemetry in queueLeadOutreach so the Copilot
	// response shows "why blocked?" inline instead of forcing the operator
	// to run the superadmin diagnostic separately.
	RiskScore   float64
	RiskCeiling float64
}

// ClaimResult is what [Store.Claim] returns on a successful claim.
// The caller MUST thread ExecutionID all the way out to the executor
// (Chrome Extension or chromedp tab); the executor echoes it back on
// the /sent or /failed callback. The server's terminal-state CAS
// gates on a match — see [Store.Finalize].
type ClaimResult struct {
	// ExecutionID is the per-attempt idempotency token. Opaque hex
	// string; opaque to callers but unique per claim across the
	// process lifetime.
	ExecutionID string
	// LeaseExpiry is the wall-clock deadline after which
	// [Store.ResetStaleExecuting] is allowed to steal the row back to
	// planned. Slow executions that need more time should be granted
	// a longer lease at claim time (passed via leaseDuration argument)
	// — there is intentionally no extend-lease path so a wedged
	// executor cannot keep a row pinned forever.
	LeaseExpiry time.Time
}

// DefaultLease is the per-row lease window the production outbox
// handler uses unless a caller specifies otherwise. Sized for comment
// + inbox + post actions (each ~5–30s end-to-end) with ~6x headroom
// for slow networks and post-action verification settle.
const DefaultLease = 3 * time.Minute

// ActionPolicy defines coordination rules for an outbound action type.
// PR-2 (V2 staged refactor 2026-05-20) replaced hardcoded msgType
// branches with a per-(org, action_type) policy lookup. New action
// types are added by inserting a policy row, not editing dedup code.
type ActionPolicy struct {
	ID                int64
	OrgID             int64  // 0 = global default
	ActionType        string // 'comment' | 'inbox' | 'group_post' | 'profile_post' | future
	DedupScope        string // 'per_account' | 'workspace' | 'none'
	BlockOnPlanned    bool
	BlockOnExecuting  bool
	CooldownSeconds   int
	ConversationAware bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Dedup scope constants. Anything outside this enum is rejected at
// write time.
const (
	DedupScopePerAccount = "per_account"
	DedupScopeWorkspace  = "workspace"
	DedupScopeNone       = "none"
)

// TransitionType marks what kind of state change a row in
// execution_attempts represents.
type TransitionType string

const (
	TransitionPlan     TransitionType = "plan"
	TransitionClaim    TransitionType = "claim"
	TransitionFinalize TransitionType = "finalize"
	TransitionReset    TransitionType = "reset"
)

// RecordTransitionInput is the carrier shape for [Hooks.RecordTransition].
// Outbound builds this from its internal transitionInput at the hook
// boundary; the hook owner (coordination domain, today wired in
// outbound_aliases.go) translates these fields into the cross-dialect
// INSERT against execution_attempts.
//
// Fields are pre-converted to primitive/string form so the receiving
// coordination writer takes no peer-domain types. Symmetric to the
// CapsDecision pattern (Decouple-1): outbound owns the input shape;
// coordination receives primitives. See specs/PHASE_5A_COORDINATION_AUDIT.md
// §4.2 for the cycle-avoidance rationale.
type RecordTransitionInput struct {
	OutboundID       int64
	OrgID            int64
	AccountID        int64
	CreatedBy        int64
	TargetURL        string
	ActionType       string
	Attempt          int
	Status           string // pre-converted from models.AttemptStatus
	Outcome          string // pre-converted from models.ExecutionOutcome
	FailureReason    string
	EvidenceJSON     string
	DOMVerified      bool
	NetworkVerified  bool
	TransitionType   string // pre-converted from outbound.TransitionType
	ExecutionID      string
	ResultingState   string // pre-converted from models.ExecutionState
	ResultingOutcome string // pre-converted from models.VerificationOutcome
	LeaseExpiry      sql.NullTime
}
