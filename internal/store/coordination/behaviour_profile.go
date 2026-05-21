// Domain: coordination (see internal/store/DOMAINS.md)
package coordination

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store/dbutil"
)

// Coordination Plane PR-2: Per-Account Behaviour Profile substrate.
//
// Two tables, on purpose. Static identity vs high-churn runtime counters
// live in separate rows so the orchestrator's frequent runtime writes do
// not contend with profile-management writes. See
// feedback_behaviour_profile_design.md.

// utcDayKey (the canonical UTC day-key formatter for *_today counters)
// moved to dbutil.UTCDayKey in Phase 1 of STORE_SUBPACKAGE_REFACTOR.
// Callers in this file now use dbutil.UTCDayKey.

// GetAccountBehaviourProfile returns the static behaviour profile for an
// account, or nil if none is registered. Missing-profile means the queue
// layer falls back to the default trust preset (TrustWarming), so absence
// is not an error.
func (s *Store) GetAccountBehaviourProfile(ctx context.Context, accountID int64) (*models.AccountBehaviourProfile, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("account_id is required")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT account_id, org_id, trust_level, account_age_days, persona_type,
		        workspace_role, capabilities, caps_override, notes, created_at, updated_at
		 FROM account_behaviour_profiles
		 WHERE account_id = ?`, accountID,
	)
	var p models.AccountBehaviourProfile
	var trust, role, createdAt, updatedAt string
	if err := row.Scan(&p.AccountID, &p.OrgID, &trust, &p.AccountAgeDays, &p.PersonaType,
		&role, &p.Capabilities, &p.CapsOverride, &p.Notes, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.TrustLevel = models.NormalizeTrustLevel(trust)
	p.WorkspaceRole = models.WorkspaceRole(role)
	p.CreatedAt = dbutil.ParseSQLiteTime(createdAt)
	p.UpdatedAt = dbutil.ParseSQLiteTime(updatedAt)
	return &p, nil
}

// UpsertAccountBehaviourProfile creates or updates the profile row. The
// caller supplies normalized values; this method writes them verbatim plus
// the updated_at timestamp. Missing JSON columns default to "{}".
func (s *Store) UpsertAccountBehaviourProfile(ctx context.Context, p *models.AccountBehaviourProfile) error {
	if p == nil || p.AccountID <= 0 {
		return fmt.Errorf("account_id is required")
	}
	trust := models.NormalizeTrustLevel(string(p.TrustLevel))
	capabilities := strings.TrimSpace(p.Capabilities)
	if capabilities == "" {
		capabilities = "{}"
	}
	override := strings.TrimSpace(p.CapsOverride)
	if override == "" {
		override = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO account_behaviour_profiles
			(account_id, org_id, trust_level, account_age_days, persona_type,
			 workspace_role, capabilities, caps_override, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(account_id) DO UPDATE SET
			org_id           = excluded.org_id,
			trust_level      = excluded.trust_level,
			account_age_days = excluded.account_age_days,
			persona_type     = excluded.persona_type,
			workspace_role   = excluded.workspace_role,
			capabilities     = excluded.capabilities,
			caps_override    = excluded.caps_override,
			notes            = excluded.notes,
			updated_at       = CURRENT_TIMESTAMP`,
		p.AccountID, p.OrgID, string(trust), p.AccountAgeDays, p.PersonaType,
		string(p.WorkspaceRole), capabilities, override, p.Notes,
	)
	return err
}

// GetAccountRuntimeState returns the runtime counters for an account.
// Counters are rolled to "today" automatically — if the stored
// counters_day != today's UTC date, the *_today columns are returned as
// zero. The persisted row is NOT updated by this read; the rollover is
// performed on the next write via incrementRuntimeCounterTx.
//
// Missing row returns a zero-valued AccountRuntimeState for the given
// account_id rather than an error, so the policy layer treats unknown
// accounts as "no usage yet today".
func (s *Store) GetAccountRuntimeState(ctx context.Context, accountID int64) (models.AccountRuntimeState, error) {
	if accountID <= 0 {
		return models.AccountRuntimeState{}, fmt.Errorf("account_id is required")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT account_id, org_id, counters_day, comments_today, inbox_today,
		        group_posts_today, profile_posts_today, risk_score, recent_failures,
		        COALESCE(cooldown_until,''), COALESCE(last_action_at,''), updated_at
		 FROM account_runtime_state
		 WHERE account_id = ?`, accountID,
	)
	var r models.AccountRuntimeState
	var cooldown, lastAction, updatedAt string
	if err := row.Scan(&r.AccountID, &r.OrgID, &r.CountersDay, &r.CommentsToday, &r.InboxToday,
		&r.GroupPostsToday, &r.ProfilePostsToday, &r.RiskScore, &r.RecentFailures,
		&cooldown, &lastAction, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return models.AccountRuntimeState{AccountID: accountID}, nil
		}
		return models.AccountRuntimeState{}, err
	}
	if cooldown != "" {
		r.CooldownUntil = dbutil.ParseSQLiteTime(cooldown)
	}
	if lastAction != "" {
		r.LastActionAt = dbutil.ParseSQLiteTime(lastAction)
	}
	r.UpdatedAt = dbutil.ParseSQLiteTime(updatedAt)
	today := dbutil.UTCDayKey(time.Now())
	if r.CountersDay != today {
		r.CountersDay = today
		r.CommentsToday = 0
		r.InboxToday = 0
		r.GroupPostsToday = 0
		r.ProfilePostsToday = 0
	}
	return r, nil
}

// counterColumnForAction maps a queue action_type to the runtime-state
// column name to bump. Unknown types return "" — the caller must treat
// that as "no counter to increment" rather than fall through to a default.
func counterColumnForAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "comment":
		return "comments_today"
	case "inbox":
		return "inbox_today"
	case "group_post":
		return "group_posts_today"
	case "profile_post":
		return "profile_posts_today"
	default:
		return ""
	}
}

// incrementRuntimeCounterTx bumps the today-counter for an action by 1
// inside the queue transaction. It performs the day-rollover atomically:
// when counters_day != today, all *_today columns are reset to 0 first.
//
// org_id is captured here too (not on read) so newly-created rows carry
// the right tenant key without a separate seeding step.
//
// This is the ONLY mutation path for runtime counters from the queue.
// Best-effort: errors are returned so the caller can decide whether to
// fail the surrounding tx, but the design treats counter loss as
// acceptable (queue success is the source of truth, ledger is additive).
//
// Phase 5B: exported (was incrementRuntimeCounterTx) for the hooks
// closure pattern. Package-level function — no Store state required.
func IncrementCounterTx(tx *sql.Tx, orgID, accountID int64, action string) error {
	col := counterColumnForAction(action)
	if col == "" || accountID <= 0 {
		return nil
	}
	today := dbutil.UTCDayKey(time.Now())

	// Try a same-day increment first (fast path).
	res, err := tx.Exec(
		`UPDATE account_runtime_state
		   SET `+col+` = `+col+` + 1,
		       last_action_at = CURRENT_TIMESTAMP,
		       updated_at = CURRENT_TIMESTAMP
		 WHERE account_id = ? AND counters_day = ?`,
		accountID, today,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 1 {
		return nil
	}

	// Slow path: row missing OR counters_day mismatched. Roll counters to
	// today, set the action's counter to 1, leave non-counter fields
	// (risk_score, recent_failures, cooldown_until) untouched.
	zero := map[string]int{
		"comments_today":      0,
		"inbox_today":         0,
		"group_posts_today":   0,
		"profile_posts_today": 0,
	}
	zero[col] = 1

	_, err = tx.Exec(
		`INSERT INTO account_runtime_state
			(account_id, org_id, counters_day, comments_today, inbox_today,
			 group_posts_today, profile_posts_today, risk_score, recent_failures,
			 last_action_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(account_id) DO UPDATE SET
			org_id              = excluded.org_id,
			counters_day        = excluded.counters_day,
			comments_today      = excluded.comments_today,
			inbox_today         = excluded.inbox_today,
			group_posts_today   = excluded.group_posts_today,
			profile_posts_today = excluded.profile_posts_today,
			last_action_at      = CURRENT_TIMESTAMP,
			updated_at          = CURRENT_TIMESTAMP`,
		accountID, orgID, today,
		zero["comments_today"], zero["inbox_today"], zero["group_posts_today"], zero["profile_posts_today"],
	)
	return err
}

// ApplyRiskSignal updates risk_score for an account based on a typed
// signal. The default weight is taken from models.SignalWeights; callers
// may pass a non-zero customWeight to override the default for one event
// (used by future tuning loops). risk_score is clamped to [0.0, 1.0].
//
// Multi-signal API on purpose: v1 emits only failure / success, but the
// surface accepts richer signals (captcha, redirect anomaly, comment
// deletion) so future emitters plug in without schema migration.
//
// recent_failures is bumped only for failure-class signals.
func (s *Store) ApplyRiskSignal(ctx context.Context, orgID, accountID int64, signal models.RiskSignal, customWeight float64) error {
	if accountID <= 0 {
		return fmt.Errorf("account_id is required")
	}
	weight := customWeight
	if weight == 0 {
		w, ok := models.SignalWeights[signal]
		if !ok {
			return fmt.Errorf("unknown risk signal: %q", signal)
		}
		weight = w
	}

	failureBump := 0
	switch signal {
	case models.RiskSignalFailure,
		models.RiskSignalCaptcha,
		models.RiskSignalActionRejected,
		models.RiskSignalBrowserCrash,
		models.RiskSignalCommentDeleted,
		models.RiskSignalShadowRejected,
		models.RiskSignalRedirectEscape,
		models.RiskSignalBlocked,
		models.RiskSignalRateLimited:
		failureBump = 1
	case models.RiskSignalSuccess, models.RiskSignalReplyReceived, models.RiskSignalDuplicateDetected:
		failureBump = -1
	}

	// Ensure a row exists, then apply the delta. Two statements keep the
	// SQL simple; both run in autocommit so the second always sees the
	// first's result.
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO account_runtime_state (account_id, org_id, counters_day, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(account_id) DO NOTHING`,
		accountID, orgID, dbutil.UTCDayKey(time.Now()),
	); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE account_runtime_state
		    SET risk_score      = MAX(0.0, MIN(1.0, risk_score + ?)),
		        recent_failures = MAX(0, recent_failures + ?),
		        updated_at      = CURRENT_TIMESTAMP
		  WHERE account_id = ?`,
		weight, failureBump, accountID,
	)
	return err
}

// SetAccountCooldown sets the global per-account cooldown_until. Pass a
// zero time to clear it. Used by the orchestrator when an account hits
// its risk ceiling or fails a sensitive action.
func (s *Store) SetAccountCooldown(ctx context.Context, orgID, accountID int64, until time.Time) error {
	if accountID <= 0 {
		return fmt.Errorf("account_id is required")
	}
	var arg any
	if until.IsZero() {
		arg = nil
	} else {
		arg = until.UTC().Format("2006-01-02 15:04:05")
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO account_runtime_state (account_id, org_id, counters_day, cooldown_until, updated_at)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(account_id) DO UPDATE SET
			cooldown_until = excluded.cooldown_until,
			updated_at     = CURRENT_TIMESTAMP`,
		accountID, orgID, dbutil.UTCDayKey(time.Now()), arg,
	); err != nil {
		return err
	}
	return nil
}

// ResolveAccountCaps is the convenience wrapper most callers want: load
// the static profile (or fall back to defaults) and overlay the
// per-account caps_override. Returns the trust level used so callers
// (and tests) can verify which preset they got.
func (s *Store) ResolveAccountCaps(ctx context.Context, accountID int64) (models.BehaviourCaps, models.TrustLevel, error) {
	p, err := s.GetAccountBehaviourProfile(ctx, accountID)
	if err != nil {
		return models.BehaviourCaps{}, "", err
	}
	if p == nil {
		caps := models.ResolveBehaviourCaps(models.TrustWarming, "")
		return caps, models.TrustWarming, nil
	}
	caps := models.ResolveBehaviourCaps(p.TrustLevel, p.WorkspaceRole)
	caps = models.OverlayCaps(caps, p.CapsOverride)
	return caps, p.TrustLevel, nil
}
