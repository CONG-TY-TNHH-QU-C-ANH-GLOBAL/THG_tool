package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// InsertOutboundMessage creates a new outbound message in the queue.
//
// Direct callers (admin manual draft, dashboard "approve & send") may use
// this. Agent / AI / Telegram code paths MUST go through
// QueueOutboundForOrg instead so the dedup guard, cooldown and
// store-layer approval policy run atomically.
func (s *Store) InsertOutboundMessage(msg *models.OutboundMessage) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, target_name, content, context, image_path, status, ai_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.OrgID, msg.Type, msg.Platform, msg.AccountID, msg.TargetURL, msg.TargetName, msg.Content, msg.Context, msg.ImagePath, msg.Status, msg.AIModel,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// IsAutoOutboundEnabledForOrg reports whether the organization has opted
// into immediate-execution outbound. The flag lives in user_context under
// the org-scoped key `org:{id}:outbound_mode` and is admin-controlled —
// LLM tools must NOT be able to flip it. Any value other than the literal
// "auto" (case-insensitive) leaves the org in the safe default
// (approval-required).
//
// This helper is the single source of truth — never inline the lookup.
func (s *Store) IsAutoOutboundEnabledForOrg(orgID int64) bool {
	if orgID <= 0 {
		return false
	}
	value, err := s.GetContext(fmt.Sprintf("org:%d:outbound_mode", orgID))
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "auto")
}

// OutboundQueueResult carries the queue-level outcome back to the caller
// alongside the new row ID.
type OutboundQueueResult struct {
	ID       int64
	Status   models.OutboundStatus
	Decision OutboundGuardDecision
}

// QueueOutboundForOrg is the canonical write path for AI / agent /
// Telegram code that produces outbound messages. It performs all three
// production guards atomically inside a single transaction:
//
//  1. CanQueueOutboundForOrg — dedup, cooldown, conversation thread state.
//  2. Store-layer approval policy — the caller's requestedAuto is honoured
//     ONLY if the org's outbound_mode flag is actually "auto". This blocks
//     prompt-injection attacks where an LLM tool call sets auto=true even
//     though the org is supposed to be approval-required.
//  3. The partial UNIQUE index on (org_id, type, target_url) for active
//     statuses — the final fail-safe if two transactions race past the
//     application-level guard.
//
// Returns OutboundQueueResult.Decision.Allowed=false (with ID=0) when the
// guard blocked the write. The caller should propagate Reason to the user
// (e.g. "duplicate_outbound_target") instead of treating it as an error.
//
// Returns a non-nil error only on unexpected DB failures or constraint
// violations (which indicate a race we should learn from — log + retry
// the guard once).
func (s *Store) QueueOutboundForOrg(msg *models.OutboundMessage, requestedAuto bool, cooldown time.Duration) (OutboundQueueResult, error) {
	if msg == nil || msg.OrgID <= 0 {
		return OutboundQueueResult{}, fmt.Errorf("org_id is required")
	}
	if strings.TrimSpace(msg.TargetURL) == "" {
		return OutboundQueueResult{}, fmt.Errorf("target_url is required")
	}

	// Retry the whole transaction on SQLite busy errors. A single attempt
	// is enough for the application-level guard, but under concurrent
	// writers SQLite can return SQLITE_BUSY when a deferred transaction
	// upgrades to a writer after another writer just committed —
	// busy_timeout alone does not cover that snapshot conflict. retryOnBusy
	// short-circuits immediately for any non-busy error.
	var result OutboundQueueResult
	err := retryOnBusy(7, func() error {
		var attemptErr error
		result, attemptErr = s.queueOutboundForOrgOnce(msg, requestedAuto, cooldown)
		return attemptErr
	})
	return result, err
}

func (s *Store) queueOutboundForOrgOnce(msg *models.OutboundMessage, requestedAuto bool, cooldown time.Duration) (OutboundQueueResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return OutboundQueueResult{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	guard, err := s.canQueueOutboundTx(tx, msg.OrgID, msg.AccountID, string(msg.Type), msg.TargetURL, msg.TargetURL, cooldown)
	if err != nil {
		return OutboundQueueResult{}, err
	}
	if !guard.Allowed {
		return OutboundQueueResult{Decision: guard}, nil
	}

	// AUTONOMOUS-VERIFIED-EXECUTION (project goal, May-2026): the
	// system no longer maintains an approval / draft gate. Every
	// queued outbound flows directly to the planned/executable
	// state. The legacy outbound_mode='draft' org policy and the
	// requestedAuto argument are kept on the signature for caller
	// compatibility during the rollout window but are no longer
	// consulted — the autonomous-first model treats human approval
	// as a UX layer that operators can opt into at the dashboard
	// (e.g. by pausing the executor) rather than a server-side
	// gate.
	_ = requestedAuto
	status := models.OutboundApproved
	msg.Status = status

	res, err := tx.Exec(
		`INSERT INTO outbound_messages (org_id, type, platform, account_id, target_url, target_name, content, context, image_path, status, ai_model)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.OrgID, msg.Type, msg.Platform, msg.AccountID, msg.TargetURL, msg.TargetName, msg.Content, msg.Context, msg.ImagePath, msg.Status, msg.AIModel,
	)
	if err != nil {
		// Likely UNIQUE collision under concurrency — surface as a guard
		// reason rather than an opaque DB error. Detect this BEFORE the
		// busy-error path so a deterministic UNIQUE collision doesn't get
		// misread as a transient lock and retried (which would pointlessly
		// rewalk the guard 7 times).
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return OutboundQueueResult{Decision: OutboundGuardDecision{
				Allowed: false, Reason: "duplicate_outbound_target_race",
			}}, nil
		}
		return OutboundQueueResult{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return OutboundQueueResult{}, err
	}

	// Coordination Plane: record this attempt in the action ledger. Ledger
	// failure must NOT fail the queue — the outbound row is already written.
	if ledgerErr := recordActionLedgerTx(tx, msg.OrgID, msg.AccountID, string(msg.Type), msg.TargetURL, id, cooldown); ledgerErr != nil {
		// Best-effort log; do not roll back the outbound row.
		// The ledger is additive coordination data, not the source of truth.
		_ = ledgerErr
	}

	// Coordination Plane PR-2: bump the per-account daily counter for
	// this action type. Same best-effort policy as the ledger — the
	// queue is the source of truth, counters are policy data.
	if counterErr := incrementRuntimeCounterTx(tx, msg.OrgID, msg.AccountID, string(msg.Type)); counterErr != nil {
		_ = counterErr
	}

	if err := tx.Commit(); err != nil {
		return OutboundQueueResult{}, err
	}
	return OutboundQueueResult{
		ID:       id,
		Status:   status,
		Decision: OutboundGuardDecision{Allowed: true, Reason: "ok"},
	}, nil
}

// canQueueOutboundTx is the transactional twin of CanQueueOutboundForOrg.
// Inlines the same logic but reads through the open transaction so the
// SELECT and the subsequent INSERT see the same snapshot under SQLite WAL.
//
// Coordination Plane scoping rule:
//   - inbox: cross-account check (3 accounts to same lead in 5min = spam — block)
//   - all other types: per-account check (3 accounts commenting same viral post
//     over the day = amplification — allow). See project_distributed_coordination.md.
//
// Coordination Plane PR-2 adds the per-account behaviour layer:
//   - account-wide cooldown_until (anti-burst / orchestrator-imposed pause)
//   - daily cap from the resolved trust-level preset
//   - risk-score ceiling
//
// Caps are looked up via the resolver (trust_level + workspace_role); the
// runtime state is read tx-aware so concurrent queue calls see the most
// recent committed counter. A small race window between SELECT and the
// counter UPDATE in the success path may allow off-by-one over-cap under
// heavy concurrency — accepted for v1.
func (s *Store) canQueueOutboundTx(tx *sql.Tx, orgID, accountID int64, msgType, targetURL, profileURL string, cooldown time.Duration) (OutboundGuardDecision, error) {
	msgType = strings.TrimSpace(strings.ToLower(msgType))
	targetURL = strings.TrimSpace(targetURL)
	profileURL = strings.TrimSpace(profileURL)
	if cooldown <= 0 {
		cooldown = 24 * time.Hour
	}

	// inbox is the one action whose dedup is workspace-wide (cross-account)
	// because multiple staff messaging the same lead is the spam-cluster case
	// the Coordination Plane explicitly prevents. Every other action type is
	// per-account: Alice having a pending comment on post X does not stop
	// Bob from commenting on post X.
	crossAccount := msgType == "inbox"

	query := `SELECT id, status, COALESCE(sent_at, created_at)
		 FROM outbound_messages
		 WHERE org_id = ? AND type = ? AND target_url = ?
		   AND status NOT IN ('failed','rejected')`
	args := []any{orgID, msgType, targetURL}
	if !crossAccount {
		query += ` AND account_id = ?`
		args = append(args, accountID)
	}
	query += ` ORDER BY created_at DESC LIMIT 1`

	var existingID int64
	var status string
	var createdAt string
	err := tx.QueryRow(query, args...).Scan(&existingID, &status, &createdAt)
	if err != nil && err != sql.ErrNoRows {
		return OutboundGuardDecision{}, err
	}
	if err == nil {
		lastAt := parseSQLiteTime(createdAt)
		reason := "duplicate_outbound_target"
		if !crossAccount {
			reason = "duplicate_outbound_per_account"
		}
		if msgType == "comment" || status == string(models.OutboundDraft) || status == string(models.OutboundApproved) {
			return OutboundGuardDecision{Allowed: false, Reason: reason, ExistingID: existingID, LastOutboundAt: lastAt}, nil
		}
		if time.Since(lastAt) < cooldown {
			return OutboundGuardDecision{Allowed: false, Reason: "outbound_cooldown_active", ExistingID: existingID, LastOutboundAt: lastAt}, nil
		}
	}

	if msgType == "inbox" {
		if profileURL == "" {
			profileURL = targetURL
		}
		if thread, err := s.GetThreadByProfileForOrg(orgID, profileURL); err == nil && thread != nil {
			if thread.Status == "closed" || thread.Status == "converted" {
				return OutboundGuardDecision{Allowed: false, Reason: "conversation_closed", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
			if !thread.LastInboundAt.IsZero() && thread.LastInboundAt.After(thread.LastOutboundAt) {
				return OutboundGuardDecision{Allowed: true, Reason: "lead_replied", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
			if !thread.LastOutboundAt.IsZero() && time.Since(thread.LastOutboundAt) < cooldown {
				return OutboundGuardDecision{Allowed: false, Reason: "awaiting_reply_cooldown", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
		} else if err != nil && err != sql.ErrNoRows {
			return OutboundGuardDecision{}, err
		}
	}

	// Behaviour Profile checks: account cooldown + daily cap + risk ceiling.
	// Account_id == 0 means a legacy / unowned queue path — skip the
	// behaviour layer entirely (no profile to check against).
	if accountID > 0 {
		if guard, err := s.checkBehaviourCapsTx(tx, accountID, msgType); err != nil {
			return OutboundGuardDecision{}, err
		} else if !guard.Allowed {
			return guard, nil
		}
	}

	return OutboundGuardDecision{Allowed: true, Reason: "ok"}, nil
}

// checkBehaviourCapsTx runs the Coordination Plane PR-2 enforcement layer
// against an open queue transaction. Reasons returned to the caller:
//   - account_cooldown_active        : cooldown_until is in the future
//   - daily_limit_exceeded           : today-counter has reached the cap
//   - risk_ceiling_exceeded          : risk_score >= preset ceiling
//
// Profile-missing is NOT an error — a fresh account inherits the
// TrustWarming preset.
func (s *Store) checkBehaviourCapsTx(tx *sql.Tx, accountID int64, msgType string) (OutboundGuardDecision, error) {
	caps, _, err := s.ResolveAccountCaps(context.Background(), accountID)
	if err != nil {
		return OutboundGuardDecision{}, err
	}

	// Single round-trip: read every column the cap decision needs in one
	// SELECT, then apply the day-rollover rule + cap check in Go. The prior
	// version did two reads per queue (cooldown/risk + per-action counter).
	var (
		countersDay              string
		commentsToday, inboxToday, groupPostsToday, profilePostsToday int
		riskScore                float64
		cooldownUntilStr         string
	)
	err = tx.QueryRow(
		`SELECT counters_day, comments_today, inbox_today, group_posts_today,
		        profile_posts_today, risk_score, COALESCE(cooldown_until,'')
		   FROM account_runtime_state
		  WHERE account_id = ?`, accountID,
	).Scan(&countersDay, &commentsToday, &inboxToday, &groupPostsToday,
		&profilePostsToday, &riskScore, &cooldownUntilStr)
	if err != nil && err != sql.ErrNoRows {
		return OutboundGuardDecision{}, err
	}

	if cooldownUntilStr != "" {
		until := parseSQLiteTime(cooldownUntilStr)
		if !until.IsZero() && time.Now().UTC().Before(until.UTC()) {
			return OutboundGuardDecision{
				Allowed:        false,
				Reason:         "account_cooldown_active",
				LastOutboundAt: until,
			}, nil
		}
	}

	if caps.RiskScoreCeiling > 0 && riskScore >= caps.RiskScoreCeiling {
		return OutboundGuardDecision{Allowed: false, Reason: "risk_ceiling_exceeded"}, nil
	}

	if col := counterColumnForAction(msgType); col != "" {
		cap := caps.CapForAction(msgType)
		if cap > 0 {
			// Day rollover: counters belong to today only if counters_day == today.
			// Reading a row with a stale counters_day means today's counter is 0.
			counter := 0
			if countersDay == utcDayKey(time.Now()) {
				switch col {
				case "comments_today":
					counter = commentsToday
				case "inbox_today":
					counter = inboxToday
				case "group_posts_today":
					counter = groupPostsToday
				case "profile_posts_today":
					counter = profilePostsToday
				}
			}
			if counter >= cap {
				return OutboundGuardDecision{Allowed: false, Reason: "daily_limit_exceeded"}, nil
			}
		}
	}

	return OutboundGuardDecision{Allowed: true, Reason: "ok"}, nil
}

// OutboundGuardDecision is the queue-level safety check result for automated
// comments/inbox messages. AI can propose actions, but this guard is the final
// production gate before anything reaches an executable outbox state.
type OutboundGuardDecision struct {
	Allowed        bool
	Reason         string
	ExistingID     int64
	LastOutboundAt time.Time
	LastInboundAt  time.Time
}

// CanQueueOutboundForOrg prevents repeated comments/messages against the same
// post/profile unless the lead has replied and the conversation needs service.
func (s *Store) CanQueueOutboundForOrg(orgID int64, msgType, targetURL, profileURL string, cooldown time.Duration) (OutboundGuardDecision, error) {
	msgType = strings.TrimSpace(strings.ToLower(msgType))
	targetURL = strings.TrimSpace(targetURL)
	profileURL = strings.TrimSpace(profileURL)
	if targetURL == "" {
		return OutboundGuardDecision{Allowed: false, Reason: "missing_target_url"}, nil
	}
	if cooldown <= 0 {
		cooldown = 24 * time.Hour
	}

	var existingID int64
	var status string
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, status, COALESCE(sent_at, created_at)
		 FROM outbound_messages
		 WHERE org_id = ? AND type = ? AND target_url = ?
		   AND status NOT IN ('failed','rejected')
		 ORDER BY created_at DESC LIMIT 1`,
		orgID, msgType, targetURL,
	).Scan(&existingID, &status, &createdAt)
	if err != nil && err != sql.ErrNoRows {
		return OutboundGuardDecision{}, err
	}
	if err == nil {
		lastAt := parseSQLiteTime(createdAt)
		if msgType == "comment" || status == string(models.OutboundDraft) || status == string(models.OutboundApproved) {
			return OutboundGuardDecision{Allowed: false, Reason: "duplicate_outbound_target", ExistingID: existingID, LastOutboundAt: lastAt}, nil
		}
		if time.Since(lastAt) < cooldown {
			return OutboundGuardDecision{Allowed: false, Reason: "outbound_cooldown_active", ExistingID: existingID, LastOutboundAt: lastAt}, nil
		}
	}

	if msgType == "inbox" {
		if profileURL == "" {
			profileURL = targetURL
		}
		if thread, err := s.GetThreadByProfileForOrg(orgID, profileURL); err == nil && thread != nil {
			if thread.Status == "closed" || thread.Status == "converted" {
				return OutboundGuardDecision{Allowed: false, Reason: "conversation_closed", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
			if !thread.LastInboundAt.IsZero() && thread.LastInboundAt.After(thread.LastOutboundAt) {
				return OutboundGuardDecision{Allowed: true, Reason: "lead_replied", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
			if !thread.LastOutboundAt.IsZero() && time.Since(thread.LastOutboundAt) < cooldown {
				return OutboundGuardDecision{Allowed: false, Reason: "awaiting_reply_cooldown", LastOutboundAt: thread.LastOutboundAt, LastInboundAt: thread.LastInboundAt}, nil
			}
		} else if err != nil && err != sql.ErrNoRows {
			return OutboundGuardDecision{}, err
		}
	}

	return OutboundGuardDecision{Allowed: true, Reason: "ok"}, nil
}

// GetOutboundByStatus returns outbound messages filtered by status.
func (s *Store) GetOutboundByStatus(status string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at, COALESCE(execution_id, '')
		FROM outbound_messages`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetOutboundByStatusForOrg returns outbound messages for one tenant.
func (s *Store) GetOutboundByStatusForOrg(orgID int64, status string, limit int) ([]models.OutboundMessage, error) {
	return s.GetOutboundByFilterForOrg(orgID, status, "", limit)
}

// GetOutboundByFilter returns outbound messages filtered by optional status and/or type.
func (s *Store) GetOutboundByFilter(status, msgType string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at, COALESCE(execution_id, '')
		FROM outbound_messages`
	var args []any
	var clauses []string
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if msgType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, msgType)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetOutboundByFilterForOrg returns tenant-scoped outbound messages.
func (s *Store) GetOutboundByFilterForOrg(orgID int64, status, msgType string, limit int) ([]models.OutboundMessage, error) {
	query := `SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at, COALESCE(execution_id, '')
		FROM outbound_messages`
	var args []any
	clauses := []string{"org_id = ?"}
	args = append(args, orgID)
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	if msgType != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, msgType)
	}
	query += " WHERE " + strings.Join(clauses, " AND ")
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetSentGroupPosts returns group_post messages that were successfully sent (within last N days).
func (s *Store) GetSentGroupPosts(withinDays int) ([]models.OutboundMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
			COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at, COALESCE(execution_id, '')
		FROM outbound_messages
		WHERE type = 'group_post' AND status IN ('sent', 'approved')
		  AND created_at >= datetime('now', ?)
		ORDER BY created_at DESC`,
		fmt.Sprintf("-%d days", withinDays),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.OutboundMessage
	for rows.Next() {
		m, err := scanOutboundMessage(rows)
		if err != nil {
			continue
		}
		messages = append(messages, *m)
	}
	return messages, nil
}

// GetOutbound returns a single outbound message by ID.
func (s *Store) GetOutbound(id int64) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	err := s.db.QueryRow(
		`SELECT id, COALESCE(org_id,0), type, platform, account_id, target_url, target_name, content, context,
		COALESCE(image_path,''), status, ai_model, COALESCE(sent_at, ''), created_at, COALESCE(execution_id, '')
		FROM outbound_messages WHERE id = ?`, id,
	).Scan(&m.ID, &m.OrgID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt, &m.ExecutionID)
	if err != nil {
		return nil, err
	}
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}

// GetOutboundForOrg returns one tenant-scoped outbound message.
func (s *Store) GetOutboundForOrg(orgID, id int64) (*models.OutboundMessage, error) {
	msg, err := s.GetOutbound(id)
	if err != nil {
		return nil, err
	}
	if msg.OrgID != orgID {
		return nil, sql.ErrNoRows
	}
	return msg, nil
}

// ClaimResult is what ClaimApprovedOutboundForOrg returns on a successful
// claim. The caller MUST thread ExecutionID all the way out to the
// executor (Chrome Extension or chromedp tab); the executor echoes it
// back on the /sent or /failed callback. The server's terminal-state
// CAS gates on a match — see [Store.FinalizeOutboundAttempt].
type ClaimResult struct {
	// ExecutionID is the per-attempt idempotency token. Opaque hex
	// string; opaque to callers but unique per claim across the
	// process lifetime.
	ExecutionID string
	// LeaseExpiry is the wall-clock deadline after which
	// [Store.ResetStaleSendingOutboundForOrg] is allowed to steal the
	// row back to "approved". Slow executions that need more time
	// should be granted a longer lease at claim time (passed via
	// leaseDuration argument) — there is intentionally no extend-lease
	// path so a wedged executor cannot keep a row pinned forever.
	LeaseExpiry time.Time
}

// DefaultOutboundLease is the per-row lease window the production
// outbox handler uses unless a caller specifies otherwise. Sized for
// comment + inbox + post actions (each ~5–30s end-to-end) with ~6x
// headroom for slow networks and post-action verification settle. The
// previous global 10-min reset window is now ONLY a fallback for
// legacy rows (lease_expiry IS NULL).
const DefaultOutboundLease = 3 * time.Minute

// newExecutionID generates the per-claim idempotency token. 16
// crypto-random bytes hex-encoded — collision-free across realistic
// traffic and short enough to pass through HTTP bodies / Chrome
// message bus without padding overhead.
func newExecutionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read never fails on modern platforms, but if it
		// somehow does we still need an id. Fall back to a
		// time-derived value rather than crashing the claim path.
		return fmt.Sprintf("exec-fallback-%d", time.Now().UnixNano())
	}
	return "exec_" + hex.EncodeToString(b[:])
}

// ClaimApprovedOutboundForOrg atomically moves one approved message
// into the internal sending state, stamps a fresh execution_id, and
// sets a lease_expiry so a stuck executor cannot pin the row beyond
// its budget. Returns the (execution_id, lease_expiry) the caller
// must thread out to the executor.
//
// CAS guarantees:
//
//   - status must currently be "approved" (concurrent claimers compete
//     on this single atomic UPDATE; only one wins).
//   - org_id must match (cross-tenant defense; the row's tenant is the
//     source of truth, the caller-supplied orgID is the assertion).
//
// Backward compatibility: leaseDuration == 0 falls back to
// [DefaultOutboundLease]. workerID is normalised to a default token
// when blank.
func (s *Store) ClaimApprovedOutboundForOrg(orgID, id int64, workerID string, leaseDuration time.Duration) (*ClaimResult, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		workerID = "chrome-extension"
	}
	if leaseDuration <= 0 {
		leaseDuration = DefaultOutboundLease
	}
	execID := newExecutionID()
	leaseExpiry := time.Now().UTC().Add(leaseDuration)
	res, err := s.db.Exec(
		`UPDATE outbound_messages
		 SET status = ?, claimed_by = ?, claimed_at = CURRENT_TIMESTAMP,
		     execution_id = ?, lease_expiry = ?
		 WHERE id = ? AND org_id = ? AND status = ?`,
		models.OutboundSending, workerID, execID, leaseExpiry, id, orgID, models.OutboundApproved,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, sql.ErrNoRows
	}
	return &ClaimResult{ExecutionID: execID, LeaseExpiry: leaseExpiry}, nil
}

// FinalizeOutboundAttempt is the terminal-state CAS the agent callback
// (/sent and /failed) goes through. It encodes the execution
// idempotency invariant:
//
//   - First report with the row's current execution_id wins → status
//     flips to terminal, lease_expiry cleared. Returns finalized=true.
//   - Replayed report with the SAME execution_id when the row is
//     already terminal → returns finalized=false + the current
//     state. Handlers should treat this as success-equivalent (the
//     work already landed) — the duplicate side effects (ledger /
//     execution_attempts / risk signal) must NOT be replayed.
//   - Report carrying a DIFFERENT execution_id than the row's current
//     value → returns finalized=false + current state. Handlers
//     should 409 the request — the original execution was reset by
//     [Store.ResetStaleSendingOutboundForOrg] and re-claimed; the
//     caller's report is stale.
//   - Empty executionID in the request body is allowed for backward
//     compatibility with legacy extensions: the CAS treats it as a
//     status-only check. Once all clients ship the new token, this
//     branch can be tightened.
//
// terminalStatus must be OutboundSent or OutboundFailed; other values
// return an error.
func (s *Store) FinalizeOutboundAttempt(ctx context.Context, orgID, id int64, executionID string, terminalStatus models.OutboundStatus) (finalized bool, currentStatus models.OutboundStatus, currentExecID string, err error) {
	if terminalStatus != models.OutboundSent && terminalStatus != models.OutboundFailed {
		return false, "", "", fmt.Errorf("FinalizeOutboundAttempt: terminalStatus must be sent or failed, got %q", terminalStatus)
	}

	// CAS — match by (id, org_id, status='sending') and either
	// execution_id agreement OR an empty stored execution_id (legacy
	// rows). sent_at is only set when transitioning to sent.
	const sql = `
		UPDATE outbound_messages
		SET status = ?,
		    sent_at = CASE WHEN ? = 'sent' THEN CURRENT_TIMESTAMP ELSE sent_at END,
		    claimed_by = '',
		    claimed_at = NULL,
		    lease_expiry = NULL
		WHERE id = ? AND org_id = ? AND status = ?
		  AND (execution_id = '' OR execution_id = ?)`
	res, execErr := s.db.ExecContext(ctx, sql,
		terminalStatus, terminalStatus, id, orgID, models.OutboundSending, executionID,
	)
	if execErr != nil {
		return false, "", "", execErr
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return true, terminalStatus, executionID, nil
	}

	// CAS did not finalize. Disambiguate by reading the current row.
	row := s.db.QueryRowContext(ctx,
		`SELECT status, COALESCE(execution_id,'') FROM outbound_messages WHERE id = ? AND org_id = ?`,
		id, orgID,
	)
	var rowStatus string
	if scanErr := row.Scan(&rowStatus, &currentExecID); scanErr != nil {
		return false, "", "", scanErr
	}
	return false, models.OutboundStatus(rowStatus), currentExecID, nil
}

// ResetStaleSendingOutboundForOrg returns abandoned sending rows to
// approved when their per-row lease has expired. Two paths:
//
//   - Primary (new rows): lease_expiry is non-NULL and in the past.
//     The lease was set at claim time via
//     [Store.ClaimApprovedOutboundForOrg]; once it expires the row
//     is fair game for a re-claim.
//   - Legacy (rows claimed before the lease column existed):
//     lease_expiry IS NULL. Falls back to the previous claimed_at +
//     staleAfter window so historical data still drains.
//
// Resetting CLEARS execution_id so the next claim issues a fresh
// token — any in-flight report from the abandoned attempt then
// rightly fails its execution_id CAS at finalize time and is rejected
// as stale, preventing the SW-restart-then-re-claim duplicate-comment
// bug class.
func (s *Store) ResetStaleSendingOutboundForOrg(orgID int64, staleAfter time.Duration) error {
	if orgID <= 0 {
		return nil
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	_, err := s.db.Exec(
		`UPDATE outbound_messages
		 SET status = ?, claimed_by = '', claimed_at = NULL,
		     execution_id = '', lease_expiry = NULL
		 WHERE org_id = ?
		   AND status = ?
		   AND (
		     (lease_expiry IS NOT NULL AND lease_expiry <= CURRENT_TIMESTAMP)
		     OR (lease_expiry IS NULL AND claimed_at IS NOT NULL AND claimed_at <= datetime('now', ?))
		   )`,
		models.OutboundApproved, orgID, models.OutboundSending, fmt.Sprintf("-%d seconds", int(staleAfter.Seconds())),
	)
	return err
}

// UpdateOutboundStatus updates the status of an outbound message.
func (s *Store) UpdateOutboundStatus(id int64, status models.OutboundStatus) error {
	query := `UPDATE outbound_messages SET status = ?, claimed_by = '', claimed_at = NULL WHERE id = ?`
	if status == models.OutboundSent {
		query = `UPDATE outbound_messages SET status = ?, sent_at = CURRENT_TIMESTAMP, claimed_by = '', claimed_at = NULL WHERE id = ?`
	}
	_, err := s.db.Exec(query, status, id)
	return err
}

// UpdateOutboundStatusForOrg updates status only when the message belongs to the tenant.
func (s *Store) UpdateOutboundStatusForOrg(orgID, id int64, status models.OutboundStatus) error {
	query := `UPDATE outbound_messages SET status = ?, claimed_by = '', claimed_at = NULL WHERE id = ? AND org_id = ?`
	if status == models.OutboundSent {
		query = `UPDATE outbound_messages SET status = ?, sent_at = CURRENT_TIMESTAMP, claimed_by = '', claimed_at = NULL WHERE id = ? AND org_id = ?`
	}
	res, err := s.db.Exec(query, status, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateOutboundContent updates the content of a draft message.
func (s *Store) UpdateOutboundContent(id int64, content string) error {
	_, err := s.db.Exec(`UPDATE outbound_messages SET content = ? WHERE id = ? AND status = 'draft'`, content, id)
	return err
}

// UpdateOutboundContentForOrg updates draft content only within one tenant.
func (s *Store) UpdateOutboundContentForOrg(orgID, id int64, content string) error {
	res, err := s.db.Exec(`UPDATE outbound_messages SET content = ? WHERE id = ? AND org_id = ? AND status = 'draft'`, content, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteOutbound deletes an outbound message.
func (s *Store) DeleteOutbound(id int64) error {
	_, err := s.db.Exec(`DELETE FROM outbound_messages WHERE id = ?`, id)
	return err
}

// DeleteOutboundForOrg deletes an outbound message only within one tenant.
func (s *Store) DeleteOutboundForOrg(orgID, id int64) error {
	res, err := s.db.Exec(`DELETE FROM outbound_messages WHERE id = ? AND org_id = ?`, id, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CountOutboundByStatus returns counts for each status.
func (s *Store) CountOutboundByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM outbound_messages GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err == nil {
			counts[status] = count
		}
	}
	return counts, nil
}

// CountOutboundByStatusForOrg returns tenant-scoped status counts.
func (s *Store) CountOutboundByStatusForOrg(orgID int64) (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM outbound_messages WHERE org_id = ? GROUP BY status`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err == nil {
			counts[status] = count
		}
	}
	return counts, nil
}

func scanOutboundMessage(rows *sql.Rows) (*models.OutboundMessage, error) {
	var m models.OutboundMessage
	var sentAt string
	err := rows.Scan(&m.ID, &m.OrgID, &m.Type, &m.Platform, &m.AccountID, &m.TargetURL, &m.TargetName,
		&m.Content, &m.Context, &m.ImagePath, &m.Status, &m.AIModel, &sentAt, &m.CreatedAt, &m.ExecutionID)
	if err != nil {
		return nil, err
	}
	if sentAt != "" {
		m.SentAt, _ = time.Parse("2006-01-02 15:04:05", sentAt)
	}
	return &m, nil
}
