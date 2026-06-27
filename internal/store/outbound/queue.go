package outbound

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store/dbutil"
)

// Insert creates a new outbound message row. Low-level escape hatch
// for admin / test code; agent / AI / Telegram code paths MUST go
// through [Store.Queue] instead so the dedup guard, cooldown, and
// behaviour caps run atomically.
func (s *Store) Insert(msg *models.OutboundMessage) (int64, error) {
	if msg.ExecutionState == "" {
		msg.ExecutionState = models.ExecPlanned
	}
	var outcomeArg interface{}
	if msg.VerificationOutcome == "" {
		outcomeArg = nil
	} else {
		outcomeArg = string(msg.VerificationOutcome)
	}
	result, err := s.db.Exec(
		`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, target_name, content, context, image_path, media_path, media_type, execution_state, verification_outcome, ai_model, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.OrgID, msg.Type, msg.Platform, msg.AccountID, msg.TargetURL, msg.TargetName, msg.Content, msg.Context, msg.ImagePath, msg.MediaPath, msg.MediaType, msg.ExecutionState, outcomeArg, msg.AIModel, msg.CreatedBy,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Queue is the canonical write path for AI / agent / Telegram code
// that produces outbound messages. It performs three production
// guards atomically inside a single transaction:
//
//  1. Dedup + conversation gate via [Store.CheckDedup] (policy-driven —
//     PR-2 replaced the hardcoded msgType branches with an
//     action_policies lookup).
//  2. Behaviour caps via [Hooks.BehaviourCheck] (per-account daily
//     limit + cooldown + risk ceiling — coordination domain).
//  3. The partial UNIQUE index on (org_id, account_id, type, target_url)
//     for active executions — the final fail-safe if two transactions
//     race past the application-level guard.
//
// Returns QueueResult.Decision.Allowed=false (with ID=0) when the
// guard blocked the write. The caller should propagate Reason to the
// user (e.g. "duplicate_outbound_target") instead of treating it as
// an error.
//
// Returns a non-nil error only on unexpected DB failures or constraint
// violations (which indicate a race we should learn from — log + retry
// the guard once).
func (s *Store) Queue(msg *models.OutboundMessage, cooldown time.Duration) (QueueResult, error) {
	if msg == nil || msg.OrgID <= 0 {
		return QueueResult{}, fmt.Errorf("outbound.Queue: org_id is required")
	}
	if strings.TrimSpace(msg.TargetURL) == "" {
		return QueueResult{}, fmt.Errorf("outbound.Queue: target_url is required")
	}

	// Retry the whole transaction on SQLite busy errors. retryOnBusy
	// short-circuits immediately for any non-busy error.
	var result QueueResult
	err := dbutil.RetryOnBusy(7, func() error {
		var attemptErr error
		result, attemptErr = s.queueOnce(msg, cooldown)
		return attemptErr
	})
	return result, err
}

func (s *Store) queueOnce(msg *models.OutboundMessage, cooldown time.Duration) (QueueResult, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return QueueResult{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	guard, err := s.checkQueueGate(ctx, tx, msg.OrgID, msg.AccountID, string(msg.Type), msg.TargetURL, msg.TargetURL)
	if err != nil {
		return QueueResult{}, err
	}
	if !guard.Allowed {
		// Typed event: gate rejected the queue. Closes the diagnostic
		// gap "why didn't this lead get queued?" — production debug
		// for autocomment_redirect-class investigations.
		events.Info(ctx, events.OutboundQueueRejected,
			events.FieldOrgID, msg.OrgID,
			events.FieldAccountID, msg.AccountID,
			events.FieldActionType, string(msg.Type),
			events.FieldTargetURL, msg.TargetURL,
			events.FieldReason, guard.Reason,
		)
		return QueueResult{Decision: guard}, nil
	}

	msg.ExecutionState = models.ExecPlanned
	msg.VerificationOutcome = ""

	res, err := tx.Exec(
		`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, target_name, content, context, image_path, media_path, media_type, execution_state, verification_outcome, ai_model, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		msg.OrgID, msg.Type, msg.Platform, msg.AccountID, msg.TargetURL, msg.TargetName, msg.Content, msg.Context, msg.ImagePath, msg.MediaPath, msg.MediaType, msg.ExecutionState, msg.AIModel, msg.CreatedBy,
	)
	if err != nil {
		// Likely UNIQUE collision under concurrency — surface as a
		// guard reason rather than an opaque DB error.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return QueueResult{Decision: GuardDecision{
				Allowed: false, Reason: "duplicate_outbound_target_race",
			}}, nil
		}
		return QueueResult{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return QueueResult{}, err
	}

	// Cross-domain side effects via Hooks (best-effort).
	// tenant-ok: cross-domain projection (outbound -> coordination).
	if s.hooks.RecordActionLedger != nil {
		s.hooks.RecordActionLedger(tx, msg.OrgID, msg.AccountID, msg.CreatedBy, string(msg.Type), msg.TargetURL, id, cooldown)
	}
	if s.hooks.IncrementCounter != nil {
		s.hooks.IncrementCounter(tx, msg.OrgID, msg.AccountID, string(msg.Type))
	}

	// 'plan' transition into execution_attempts ledger (own domain side).
	s.appendTransition(ctx, tx, transitionInput{
		OutboundID:     id,
		OrgID:          msg.OrgID,
		AccountID:      msg.AccountID,
		CreatedBy:      msg.CreatedBy,
		TargetURL:      msg.TargetURL,
		ActionType:     string(msg.Type),
		Attempt:        1,
		TransitionType: TransitionPlan,
		ResultingState: models.ExecPlanned,
	})

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}

	// Typed event: queue succeeded. Closes the diagnostic gap
	// "did this lead get queued at all?" — required server-side signal
	// before any extension claim happens.
	events.Info(ctx, events.OutboundQueued,
		events.FieldOrgID, msg.OrgID,
		events.FieldOutboundID, id,
		events.FieldAccountID, msg.AccountID,
		events.FieldActionType, string(msg.Type),
		events.FieldTargetURL, msg.TargetURL,
	)

	return QueueResult{
		ID:             id,
		ExecutionState: msg.ExecutionState,
		Decision:       GuardDecision{Allowed: true, Reason: "ok"},
	}, nil
}

// IsAutoEnabledForOrg reports whether the organization has opted into
// immediate-execution outbound. The flag lives in user_context under
// the org-scoped key `org:{id}:outbound_mode` and is admin-controlled
// — LLM tools must NOT be able to flip it. Any value other than the
// literal "auto" (case-insensitive) leaves the org in the safe default
// (approval-required).
//
// This helper is the single source of truth — never inline the lookup.
//
// tenant-ok: cross-domain projection (outbound -> users/context). The
// user_context table is read-only from this domain's perspective.
func (s *Store) IsAutoEnabledForOrg(orgID int64) bool {
	if orgID <= 0 {
		return false
	}
	var value string
	key := fmt.Sprintf("org:%d:outbound_mode", orgID)
	row := s.db.QueryRow(`SELECT value FROM user_context WHERE key = ?`, key)
	if err := row.Scan(&value); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "auto")
}
