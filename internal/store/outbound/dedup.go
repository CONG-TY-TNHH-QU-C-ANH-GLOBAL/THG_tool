package outbound

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/thg/scraper/internal/store/dbutil"
)

// CheckDedup is the policy-driven dedup gate. Decision matrix is
// fully described by the (orgID, actionType) action_policies row:
//
//   - DedupScope controls whether existing-row lookup filters by
//     account_id ('per_account') or scans the whole workspace
//     ('workspace') or skips ('none').
//   - BlockOnPlanned / BlockOnExecuting toggle the in-flight gates.
//   - CooldownSeconds is the minimum gap between two enqueues of the
//     same target once the prior row has finished.
//   - ConversationAware delegates to the [Hooks.ConversationGate]
//     callback (threads domain) — inbox-style flows respect thread
//     state + reply chain.
//
// CheckDedup handles only dedup + conversation semantics. The
// behaviour-caps gate (account cooldown / daily cap / risk ceiling)
// is applied by the caller via [Hooks.BehaviourCheck] — see
// [Store.checkQueueGate] which composes both.
//
// Tx-aware so callers running inside an enqueue transaction see a
// consistent snapshot of the policy and the existing-row check.
func (s *Store) CheckDedup(
	ctx context.Context,
	tx *sql.Tx,
	orgID, accountID int64,
	actionType, targetURL, profileURL string,
) (GuardDecision, error) {
	actionType = strings.TrimSpace(strings.ToLower(actionType))
	targetURL = strings.TrimSpace(targetURL)
	profileURL = strings.TrimSpace(profileURL)

	policy, err := s.GetPolicyTx(ctx, tx, orgID, actionType)
	if err != nil {
		if err == sql.ErrNoRows {
			// Hard refusal — no policy means we don't know how to
			// coordinate this action type. Fail closed rather than
			// guessing.
			return GuardDecision{
				Allowed: false,
				Reason:  "action_policy_missing",
			}, nil
		}
		return GuardDecision{}, err
	}

	// Step 1: in-flight + recent-finalize lookup, scoped per the policy.
	if policy.DedupScope != DedupScopeNone {
		decision, err := s.checkExistingRow(ctx, tx, *policy, orgID, accountID, targetURL)
		if err != nil {
			return GuardDecision{}, err
		}
		if !decision.Allowed {
			return decision, nil
		}
	}

	// Step 2: conversation-aware gate (inbox-style) via cross-domain
	// hook (threads domain).
	// tenant-ok: cross-domain projection (outbound -> threads).
	if policy.ConversationAware && s.hooks.ConversationGate != nil {
		decision, err := s.hooks.ConversationGate(ctx, orgID, targetURL, profileURL, time.Duration(policy.CooldownSeconds)*time.Second)
		if err != nil {
			return GuardDecision{}, err
		}
		if !decision.Allowed {
			return decision, nil
		}
	}

	return GuardDecision{Allowed: true, Reason: "ok"}, nil
}

// checkExistingRow queries outbound_messages for a recent row matching
// (orgID, actionType, targetURL) under the policy's dedup_scope.
// Returns a guard decision applying block_on_planned / block_on_executing
// / cooldown rules.
func (s *Store) checkExistingRow(
	ctx context.Context,
	tx *sql.Tx,
	policy ActionPolicy,
	orgID, accountID int64,
	targetURL string,
) (GuardDecision, error) {
	query := `SELECT id, execution_state, COALESCE(sent_at, created_at)
		FROM outbound_messages
		WHERE org_id = ? AND type = ? AND target_url = ?
		  AND execution_state IN ('planned','executing','finished')`
	args := []interface{}{orgID, policy.ActionType, targetURL}
	if policy.DedupScope == DedupScopePerAccount {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY created_at DESC LIMIT 1`

	var (
		existingID int64
		execState  string
		createdAt  string
	)
	err := tx.QueryRowContext(ctx, query, args...).Scan(&existingID, &execState, &createdAt)
	if err == sql.ErrNoRows {
		return GuardDecision{Allowed: true, Reason: "ok"}, nil
	}
	if err != nil {
		return GuardDecision{}, err
	}

	lastAt := dbutil.ParseSQLiteTime(createdAt)
	reason := dedupReason(policy.DedupScope)

	if policy.BlockOnPlanned && execState == "planned" {
		return GuardDecision{
			Allowed:        false,
			Reason:         reason,
			ExistingID:     existingID,
			LastOutboundAt: lastAt,
		}, nil
	}
	if policy.BlockOnExecuting && execState == "executing" {
		return GuardDecision{
			Allowed:        false,
			Reason:         reason,
			ExistingID:     existingID,
			LastOutboundAt: lastAt,
		}, nil
	}
	// Finished rows respect the cooldown window. CooldownSeconds=0
	// disables the time check entirely.
	if execState == "finished" && policy.CooldownSeconds > 0 {
		if time.Since(lastAt) < time.Duration(policy.CooldownSeconds)*time.Second {
			return GuardDecision{
				Allowed:        false,
				Reason:         "outbound_cooldown_active",
				ExistingID:     existingID,
				LastOutboundAt: lastAt,
			}, nil
		}
	}
	return GuardDecision{Allowed: true, Reason: "ok"}, nil
}

// dedupReason maps the policy scope to the operator-facing reason
// string. Centralised so the dashboard always shows a value that
// matches what was actually checked.
func dedupReason(scope string) string {
	switch scope {
	case DedupScopePerAccount:
		return "duplicate_outbound_per_account"
	case DedupScopeWorkspace:
		return "duplicate_outbound_target"
	default:
		return "duplicate_outbound_target"
	}
}

// checkQueueGate is the composite gate the queue path uses: dedup
// (own domain) + behaviour caps (cross-domain via hook). It delegates
// to [Store.CheckDedup] and [Hooks.BehaviourCheck] so a single bug
// fix lands in both the tx-write path and the read-side preflight.
func (s *Store) checkQueueGate(ctx context.Context, tx *sql.Tx, orgID, accountID int64, msgType, targetURL, profileURL string) (GuardDecision, error) {
	guard, err := s.CheckDedup(ctx, tx, orgID, accountID, msgType, targetURL, profileURL)
	if err != nil {
		return GuardDecision{}, err
	}
	if !guard.Allowed {
		return guard, nil
	}

	// Behaviour Profile checks: account cooldown + daily cap + risk
	// ceiling. accountID == 0 means a legacy / unowned queue path —
	// skip the behaviour layer entirely (no profile to check against).
	// tenant-ok: cross-domain projection (outbound -> coordination).
	if accountID > 0 && s.hooks.BehaviourCheck != nil {
		if bGuard, err := s.hooks.BehaviourCheck(tx, accountID, msgType); err != nil {
			return GuardDecision{}, err
		} else if !bGuard.Allowed {
			return bGuard, nil
		}
	}
	return guard, nil
}

// PreflightCheck is the non-tx read-side guard used by the HTTP
// "can-I-enqueue?" preflight endpoints. Opens a read-only tx and
// runs the dedup gate (no behaviour-caps subset since preflight does
// not pin an accountID).
func (s *Store) PreflightCheck(orgID int64, msgType, targetURL, profileURL string) (GuardDecision, error) {
	if strings.TrimSpace(targetURL) == "" {
		return GuardDecision{Allowed: false, Reason: "missing_target_url"}, nil
	}

	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return GuardDecision{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	// accountID=0 — preflight doesn't have a specific actor account in
	// scope. The per-account dedup filter in CheckDedup still works
	// (falls back to workspace scope effectively), and the behaviour
	// caps subset that requires an account is skipped by the tx-path
	// guard at enqueue time.
	return s.CheckDedup(ctx, tx, orgID, 0, msgType, targetURL, profileURL)
}
