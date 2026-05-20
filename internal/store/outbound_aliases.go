// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/outbound"
)

// outbound_aliases.go — the bridge layer between the legacy
// top-level *Store API and the [outbound] subpackage. Created by
// Phase 2 of STORE_SUBPACKAGE_REFACTOR (2026-05-21).
//
// Purpose: zero caller migration. Existing call sites
// (`s.QueueOutboundForOrg(...)`, `s.ClaimPlannedOutboundForOrg(...)`,
// `store.OutboundQueueResult{}`, etc.) keep working unchanged because
// type aliases re-export the subpackage types and the methods below
// delegate.
//
// L2 invariant (binding): these wrappers are deprecated compatibility
// shims. NEW code MUST import [outbound] directly via [Store.Outbound()].
// No new bridge methods may be added — if a subpackage method exists,
// its callers either use it via Outbound() or migrate. The wrappers
// here are scheduled for deletion when the last caller migrates.

// --- Type aliases (zero-cost source compatibility) ---

// OutboundQueueResult is an alias of [outbound.QueueResult].
//
// Deprecated: import "internal/store/outbound" and use
// [outbound.QueueResult] directly in new code.
type OutboundQueueResult = outbound.QueueResult

// OutboundGuardDecision is an alias of [outbound.GuardDecision].
//
// Deprecated: import "internal/store/outbound" and use
// [outbound.GuardDecision] directly in new code.
type OutboundGuardDecision = outbound.GuardDecision

// ClaimResult is an alias of [outbound.ClaimResult].
//
// Deprecated: import "internal/store/outbound" and use
// [outbound.ClaimResult] directly in new code.
type ClaimResult = outbound.ClaimResult

// ActionPolicy is an alias of [outbound.ActionPolicy].
//
// Deprecated: import "internal/store/outbound" and use
// [outbound.ActionPolicy] directly in new code.
type ActionPolicy = outbound.ActionPolicy

// DefaultOutboundLease re-exports [outbound.DefaultLease] for
// source compatibility.
//
// Deprecated: use [outbound.DefaultLease].
const DefaultOutboundLease = outbound.DefaultLease

// Dedup scope constants — re-exports for source compatibility.
//
// Deprecated: use the outbound package constants directly.
const (
	DedupScopePerAccount = outbound.DedupScopePerAccount
	DedupScopeWorkspace  = outbound.DedupScopeWorkspace
	DedupScopeNone       = outbound.DedupScopeNone
)

// TransitionType is an alias of [outbound.TransitionType].
//
// Deprecated: use [outbound.TransitionType] directly.
type TransitionType = outbound.TransitionType

// Transition type constants — re-exports.
//
// Deprecated: use the outbound package constants directly.
const (
	TransitionPlan     = outbound.TransitionPlan
	TransitionClaim    = outbound.TransitionClaim
	TransitionFinalize = outbound.TransitionFinalize
	TransitionReset    = outbound.TransitionReset
)

// installOutboundHooks constructs the outbound subpackage Store with
// cross-domain hooks pointing at the legacy top-level helpers. Called
// once by [New] / [newSQLite] / [newPostgres] after migrations.
//
// L3 invariant (tx threading): every hook accepts the parent tx
// passed by outbound's queue path so all writes commit atomically.
// Hooks never open their own transactions.
func (s *Store) installOutboundHooks() {
	s.outbound = outbound.NewStore(s.db, s.dialect, outbound.Hooks{
		BehaviourCheck: func(tx *sql.Tx, accountID int64, msgType string) (outbound.GuardDecision, error) {
			return s.checkBehaviourCapsTx(tx, accountID, msgType)
		},
		ConversationGate: func(ctx context.Context, orgID int64, targetURL, profileURL string, cooldown time.Duration) (outbound.GuardDecision, error) {
			return s.conversationGateForOutbound(ctx, orgID, targetURL, profileURL, cooldown)
		},
		RecordActionLedger: func(tx *sql.Tx, orgID, accountID int64, msgType, targetURL string, outboundID int64, cooldown time.Duration) {
			// Best-effort, errors swallowed (the outbound row is the source of truth).
			_ = recordActionLedgerTx(tx, orgID, accountID, msgType, targetURL, outboundID, cooldown)
		},
		IncrementCounter: func(tx *sql.Tx, orgID, accountID int64, msgType string) {
			_ = incrementRuntimeCounterTx(tx, orgID, accountID, msgType)
		},
	})
}

// conversationGateForOutbound is the threads-domain adapter that
// outbound's CheckDedup calls when an action_policies row has
// ConversationAware=1. Reads the conversation_threads table directly
// (the threads-domain extraction is Phase 8 future work — Phase 2
// keeps the adapter here so outbound has a clean hook to consume).
//
// tenant-ok: cross-domain projection (outbound -> threads).
func (s *Store) conversationGateForOutbound(_ context.Context, orgID int64, targetURL, profileURL string, cooldown time.Duration) (outbound.GuardDecision, error) {
	if profileURL == "" {
		profileURL = targetURL
	}
	thread, err := s.GetThreadByProfileForOrg(orgID, profileURL)
	if err == sql.ErrNoRows || thread == nil {
		return outbound.GuardDecision{Allowed: true, Reason: "ok"}, nil
	}
	if err != nil {
		return outbound.GuardDecision{}, err
	}
	if thread.Status == "closed" || thread.Status == "converted" {
		return outbound.GuardDecision{
			Allowed:        false,
			Reason:         "conversation_closed",
			LastOutboundAt: thread.LastOutboundAt,
			LastInboundAt:  thread.LastInboundAt,
		}, nil
	}
	if !thread.LastInboundAt.IsZero() && thread.LastInboundAt.After(thread.LastOutboundAt) {
		return outbound.GuardDecision{
			Allowed:        true,
			Reason:         "lead_replied",
			LastOutboundAt: thread.LastOutboundAt,
			LastInboundAt:  thread.LastInboundAt,
		}, nil
	}
	if !thread.LastOutboundAt.IsZero() && time.Since(thread.LastOutboundAt) < cooldown {
		return outbound.GuardDecision{
			Allowed:        false,
			Reason:         "awaiting_reply_cooldown",
			LastOutboundAt: thread.LastOutboundAt,
			LastInboundAt:  thread.LastInboundAt,
		}, nil
	}
	return outbound.GuardDecision{Allowed: true, Reason: "ok"}, nil
}

// --- Bridge wrappers (all Deprecated per L2) ---

// QueueOutboundForOrg delegates to [outbound.Store.Queue].
//
// Deprecated: call s.Outbound().Queue(msg, cooldown) directly in
// new code. L2 forbids adding new wrappers — this one exists only
// until existing callers migrate.
func (s *Store) QueueOutboundForOrg(msg *models.OutboundMessage, cooldown time.Duration) (OutboundQueueResult, error) {
	return s.outbound.Queue(msg, cooldown)
}

// InsertOutboundMessage delegates to [outbound.Store.Insert].
//
// Deprecated: call s.Outbound().Insert(msg) directly in new code.
func (s *Store) InsertOutboundMessage(msg *models.OutboundMessage) (int64, error) {
	return s.outbound.Insert(msg)
}

// IsAutoOutboundEnabledForOrg delegates to [outbound.Store.IsAutoEnabledForOrg].
//
// Deprecated: call s.Outbound().IsAutoEnabledForOrg(orgID) directly.
func (s *Store) IsAutoOutboundEnabledForOrg(orgID int64) bool {
	return s.outbound.IsAutoEnabledForOrg(orgID)
}

// CanQueueOutboundForOrg delegates to [outbound.Store.PreflightCheck].
//
// Deprecated: call s.Outbound().PreflightCheck(orgID, ...) directly.
// The `cooldown` parameter is no longer consulted (policy-driven).
func (s *Store) CanQueueOutboundForOrg(orgID int64, msgType, targetURL, profileURL string, cooldown time.Duration) (OutboundGuardDecision, error) {
	_ = cooldown
	return s.outbound.PreflightCheck(orgID, msgType, targetURL, profileURL)
}

// ClaimPlannedOutboundForOrg delegates to [outbound.Store.Claim].
//
// Deprecated: call s.Outbound().Claim(orgID, id, workerID, lease) directly.
func (s *Store) ClaimPlannedOutboundForOrg(orgID, id int64, workerID string, leaseDuration time.Duration) (*ClaimResult, error) {
	return s.outbound.Claim(orgID, id, workerID, leaseDuration)
}

// FinalizeOutboundAttempt delegates to [outbound.Store.Finalize].
//
// Deprecated: call s.Outbound().Finalize(...) directly.
func (s *Store) FinalizeOutboundAttempt(
	ctx context.Context,
	orgID, id int64,
	executionID string,
	terminalState models.ExecutionState,
	verificationOutcome models.VerificationOutcome,
) (finalized bool, currentState models.ExecutionState, currentOutcome models.VerificationOutcome, currentExecID string, err error) {
	return s.outbound.Finalize(ctx, orgID, id, executionID, terminalState, verificationOutcome)
}

// ResetStaleExecutingForOrg delegates to [outbound.Store.ResetStaleExecuting].
//
// Deprecated: call s.Outbound().ResetStaleExecuting(orgID, after) directly.
func (s *Store) ResetStaleExecutingForOrg(orgID int64, staleAfter time.Duration) error {
	return s.outbound.ResetStaleExecuting(orgID, staleAfter)
}

// GetOutboundForOrg delegates to [outbound.Store.Get].
//
// Deprecated: call s.Outbound().Get(orgID, id) directly.
func (s *Store) GetOutboundForOrg(orgID, id int64) (*models.OutboundMessage, error) {
	return s.outbound.Get(orgID, id)
}

// GetOutboundByExecutionStateForOrg delegates to [outbound.Store.ListByState].
//
// Deprecated: call s.Outbound().ListByState(...) directly.
func (s *Store) GetOutboundByExecutionStateForOrg(orgID int64, execState models.ExecutionState, msgType string, limit int) ([]models.OutboundMessage, error) {
	return s.outbound.ListByState(orgID, execState, msgType, limit)
}

// GetOutboundByFilterForOrg delegates to [outbound.Store.ListByLegacyStatus].
//
// Deprecated: call s.Outbound().ListByLegacyStatus(...) directly.
func (s *Store) GetOutboundByFilterForOrg(orgID int64, legacyStatus, msgType string, limit int) ([]models.OutboundMessage, error) {
	return s.outbound.ListByLegacyStatus(orgID, legacyStatus, msgType, limit)
}

// GetOutboundByStatusForOrg delegates to [outbound.Store.ListByLegacyStatus]
// with empty type filter.
//
// Deprecated: call s.Outbound().ListByLegacyStatus(orgID, status, "", limit) directly.
func (s *Store) GetOutboundByStatusForOrg(orgID int64, status string, limit int) ([]models.OutboundMessage, error) {
	return s.outbound.ListByLegacyStatus(orgID, status, "", limit)
}

// GetSentGroupPostsForOrg delegates to [outbound.Store.VerifiedGroupPostsWithin].
//
// Deprecated: call s.Outbound().VerifiedGroupPostsWithin(orgID, days) directly.
func (s *Store) GetSentGroupPostsForOrg(orgID int64, withinDays int) ([]models.OutboundMessage, error) {
	return s.outbound.VerifiedGroupPostsWithin(orgID, withinDays)
}

// CountOutboundByStatusForOrg delegates to [outbound.Store.CountByState].
//
// Deprecated: call s.Outbound().CountByState(orgID) directly.
func (s *Store) CountOutboundByStatusForOrg(orgID int64) (map[string]int, error) {
	return s.outbound.CountByState(orgID)
}

// UpdateOutboundContentForOrg delegates to [outbound.Store.EditContent].
//
// Deprecated: call s.Outbound().EditContent(orgID, id, content) directly.
func (s *Store) UpdateOutboundContentForOrg(orgID, id int64, content string) error {
	return s.outbound.EditContent(orgID, id, content)
}

// DeleteOutboundForOrg delegates to [outbound.Store.Delete].
//
// Deprecated: call s.Outbound().Delete(orgID, id) directly.
func (s *Store) DeleteOutboundForOrg(orgID, id int64) error {
	return s.outbound.Delete(orgID, id)
}

// GetActionPolicy delegates to [outbound.Store.GetPolicy].
//
// Deprecated: call s.Outbound().GetPolicy(ctx, orgID, actionType) directly.
func (s *Store) GetActionPolicy(ctx context.Context, orgID int64, actionType string) (*ActionPolicy, error) {
	return s.outbound.GetPolicy(ctx, orgID, actionType)
}

// GetActionPolicyTx delegates to [outbound.Store.GetPolicyTx].
//
// Deprecated: call s.Outbound().GetPolicyTx(ctx, tx, orgID, actionType) directly.
func (s *Store) GetActionPolicyTx(ctx context.Context, tx *sql.Tx, orgID int64, actionType string) (*ActionPolicy, error) {
	return s.outbound.GetPolicyTx(ctx, tx, orgID, actionType)
}

// UpsertActionPolicy delegates to [outbound.Store.UpsertPolicy].
//
// Deprecated: call s.Outbound().UpsertPolicy(ctx, p) directly.
func (s *Store) UpsertActionPolicy(ctx context.Context, p ActionPolicy) error {
	return s.outbound.UpsertPolicy(ctx, p)
}

// ListActionPoliciesForOrg delegates to [outbound.Store.ListPoliciesForOrg].
//
// Deprecated: call s.Outbound().ListPoliciesForOrg(ctx, orgID) directly.
func (s *Store) ListActionPoliciesForOrg(ctx context.Context, orgID int64) ([]ActionPolicy, error) {
	return s.outbound.ListPoliciesForOrg(ctx, orgID)
}

// CheckDedupTx delegates to [outbound.Store.CheckDedup].
//
// Deprecated: call s.Outbound().CheckDedup(ctx, tx, ...) directly.
func (s *Store) CheckDedupTx(ctx context.Context, tx *sql.Tx, orgID, accountID int64, actionType, targetURL, profileURL string) (OutboundGuardDecision, error) {
	return s.outbound.CheckDedup(ctx, tx, orgID, accountID, actionType, targetURL, profileURL)
}
