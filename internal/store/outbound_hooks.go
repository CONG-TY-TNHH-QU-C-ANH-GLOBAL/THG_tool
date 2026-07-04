// Domain: outbound (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store/coordination"
	"github.com/thg/scraper/internal/store/outbound"
)

// outbound_hooks.go — the composition point where the top-level *Store
// wires the [outbound] subpackage to its sibling domains (coordination,
// threads, extension gate). This is real cross-domain wiring, not a
// compatibility layer: the deprecated alias/wrapper bridge that used to
// live here (outbound_aliases.go) was dissolved once the last caller
// migrated to [Store.Outbound()] and the agent lifecycle port re-anchored
// at [outbound.Store] directly.

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
			// Adapter boundary: coordination returns CapsDecision (a
			// primitive that does NOT import outbound types — see
			// behaviour_caps_check.go for the rationale). This closure
			// converts it into outbound.GuardDecision shape.
			// CooldownUntil maps to LastOutboundAt for back-compat with
			// the existing GuardDecision consumer; the receiving layer
			// has always overloaded LastOutboundAt to carry "cooldown
			// expiry" for the account_cooldown_active reason. Preserving
			// that exact mapping is Decouple-1's mechanical guarantee.
			decision, err := s.coordination.CheckCapsTx(tx, accountID, msgType)
			if err != nil {
				return outbound.GuardDecision{}, err
			}
			if !decision.Allowed {
				return outbound.GuardDecision{
					Allowed:        false,
					Reason:         decision.Reason,
					LastOutboundAt: decision.CooldownUntil,
					RiskScore:      decision.RiskScore,
					RiskCeiling:    decision.RiskCeiling,
				}, nil
			}
			// PR-4 extension version gate: a connector running an
			// update_required/unsupported build gets NO new outbound
			// tasks. Audited as blocked_by_extension_version (skipped
			// ledger row) inside the same tx — never a silent failure.
			if s.extensionGateForOutbound(tx, accountID, msgType) {
				return outbound.GuardDecision{Allowed: false, Reason: LedgerReasonExtensionBlocked}, nil
			}
			return outbound.GuardDecision{Allowed: true, Reason: decision.Reason}, nil
		},
		ConversationGate: func(ctx context.Context, orgID int64, targetURL, profileURL string, cooldown time.Duration) (outbound.GuardDecision, error) {
			return s.conversationGateForOutbound(ctx, orgID, targetURL, profileURL, cooldown)
		},
		RecordActionLedger: func(tx *sql.Tx, in outbound.RecordLedgerInput) {
			// Best-effort, errors swallowed (the outbound row is the
			// source of truth). Failures are emitted as typed events
			// (events.ExecutionHookFailed) so the Control Plane can
			// surface them — see specs/RUNTIME_TOPOLOGY.md §5 failure
			// surface gap fixed by this emission. The carrier struct is
			// unpacked to primitives here so coordination stays free of
			// any outbound import (same pattern as RecordTransition).
			if err := coordination.RecordLedgerTx(tx, in.OrgID, in.AccountID, in.CreatedBy, in.MsgType, in.TargetURL, in.OutboundID, in.Cooldown); err != nil {
				events.Warn(context.Background(), events.ExecutionHookFailed,
					events.FieldHook, "RecordLedgerTx",
					events.FieldOrgID, in.OrgID,
					events.FieldAccountID, in.AccountID,
					events.FieldActionType, in.MsgType,
					events.FieldOutboundID, in.OutboundID,
					events.FieldErr, err,
				)
			}
		},
		IncrementCounter: func(tx *sql.Tx, orgID, accountID int64, msgType string) {
			if err := coordination.IncrementCounterTx(tx, orgID, accountID, msgType); err != nil {
				events.Warn(context.Background(), events.ExecutionHookFailed,
					events.FieldHook, "IncrementCounterTx",
					events.FieldOrgID, orgID,
					events.FieldAccountID, accountID,
					events.FieldActionType, msgType,
					events.FieldErr, err,
				)
			}
		},
		RecordTransition: func(ctx context.Context, tx *sql.Tx, in outbound.RecordTransitionInput) {
			// Unpack the carrier struct into primitives so coordination
			// stays free of any outbound import. This is the single
			// wiring point that imports both domains (Phase 5B, 2026-05-21).
			//
			// RecordTransitionTx is best-effort by design (warn-log on
			// failure inside the implementation). The hook failure itself
			// is also surfaced via events.ExecutionHookFailed inside the
			// implementation's slog.WarnContext path.
			s.coordination.RecordTransitionTx(ctx, tx,
				in.OutboundID, in.OrgID, in.AccountID, in.CreatedBy,
				in.TargetURL, in.ActionType, in.Attempt,
				in.Status, in.Outcome, in.FailureReason, in.EvidenceJSON,
				in.DOMVerified, in.NetworkVerified,
				in.TransitionType, in.ExecutionID,
				in.ResultingState, in.ResultingOutcome, in.LeaseExpiry,
			)
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
	thread, err := s.Threads().GetThreadByProfileForOrg(orgID, profileURL)
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
