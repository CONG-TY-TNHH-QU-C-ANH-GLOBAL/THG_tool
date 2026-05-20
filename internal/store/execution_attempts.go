// Domain: coordination (see internal/store/DOMAINS.md)
package store

import (
	"github.com/thg/scraper/internal/store/dbutil"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// VerificationEvidence is the JSON payload the verifier (extension or
// server-side DOM check) attaches to an execution_attempts row. Stored as
// raw JSON in evidence_json so the schema doesn't need to evolve every
// time a new proof shape is added. Helper accessor below.
type VerificationEvidence struct {
	// CommentPermalink is the canonical URL of the rendered comment node,
	// when the verifier could read it. Strongest "platform accepted" proof.
	CommentPermalink string `json:"comment_permalink,omitempty"`
	// MessageBubbleID is the DOM id (or stable selector path) of the
	// rendered message bubble in the inbox thread.
	MessageBubbleID string `json:"message_bubble_id,omitempty"`
	// DOMSnippet is a small HTML excerpt the verifier captured as proof.
	// Bounded (≤2KB) by the receive endpoint so we don't bloat the row.
	DOMSnippet string `json:"dom_snippet,omitempty"`
	// PageURLAfter is the browser's location AFTER the click. Used to
	// detect redirected_feed / context_drift outcomes.
	PageURLAfter string `json:"page_url_after,omitempty"`
	// ScreenshotPath is the local path (or object-store key) where the
	// verifier saved a screenshot of the post-submit state. Optional.
	ScreenshotPath string `json:"screenshot_path,omitempty"`
	// ObservedAt is when the verifier captured the proof. Differs from
	// finished_at when the verification window had retries.
	ObservedAt time.Time `json:"observed_at,omitempty"`
	// Notes is free-form text from the verifier (e.g. "rate-limit banner
	// detected on toast", "comment count went 12→13").
	Notes string `json:"notes,omitempty"`
}

// BeginExecutionAttempt opens a new execution_attempts row when the
// executor (extension or future server-side runner) starts working on
// an outbound. Returns the row ID for subsequent updates. attempt is
// derived (caller passes 1 for first try; retries pass N+1).
//
// The action_ledger_id may be 0 — not every action goes through the
// ledger (e.g. manual sends), and the table is the canonical attempt
// record regardless of upstream linkage.
func (s *Store) BeginExecutionAttempt(ctx context.Context, a models.ExecutionAttempt) (int64, error) {
	if a.OrgID <= 0 {
		return 0, fmt.Errorf("execution_attempts: org_id required")
	}
	if strings.TrimSpace(a.ActionType) == "" {
		return 0, fmt.Errorf("execution_attempts: action_type required")
	}
	if a.Attempt <= 0 {
		a.Attempt = 1
	}
	status := strings.TrimSpace(string(a.Status))
	if status == "" {
		status = string(models.AttemptQueued)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO execution_attempts
			(action_ledger_id, outbound_id, org_id, account_id, target_url,
			 action_type, attempt, status, outcome, failure_reason, evidence_json,
			 dom_verified, network_verified, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '', '', '{}', 0, 0, CURRENT_TIMESTAMP)`,
		a.ActionLedgerID, a.OutboundID, a.OrgID, a.AccountID, a.TargetURL,
		a.ActionType, a.Attempt, status,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishExecutionAttempt records the verified outcome of an attempt. It
// is the ONLY place the system commits the verifier's classification —
// every other table (action_ledger, outbound_messages, runtime_state)
// derives its understanding of reality from this row. Calling this with
// outcome="" is a contract violation: callers MUST classify, even if the
// classification is verification_timeout.
//
// Returns sql.ErrNoRows when attemptID does not exist (defensive — the
// caller should always have a valid ID from BeginExecutionAttempt).
func (s *Store) FinishExecutionAttempt(ctx context.Context, attemptID int64, outcome models.ExecutionOutcome, failureReason string, evidence VerificationEvidence) error {
	if attemptID <= 0 {
		return fmt.Errorf("execution_attempts: attempt_id required")
	}
	if strings.TrimSpace(string(outcome)) == "" {
		return fmt.Errorf("execution_attempts: outcome required (caller must classify even on timeout)")
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		evidenceJSON = []byte("{}")
	}
	domVerified := 0
	if outcome == models.ExecutionDOMVerified || outcome == models.ExecutionDuplicateBlocked {
		domVerified = 1
	}
	// Status terminal value: dom_verified for verified successes,
	// failed for everything else. Detail lives in outcome.
	terminalStatus := string(models.AttemptFailed)
	if domVerified == 1 {
		terminalStatus = string(models.AttemptDOMVerified)
	}
	// tenant-ok: attemptID is an internal autoincrement ID issued by
	// BeginExecutionAttempt within an org-scoped tx. Callers (verifier,
	// outbox handler) thread the ID directly; there is no API surface
	// where a tenant could submit an unknown attemptID. The defensive
	// org filter is not added here because the attempt_id is itself
	// the secret token. Listed as a v2-tenant-isolation followup if
	// the caller surface ever changes.
	res, err := s.db.ExecContext(ctx,
		`UPDATE execution_attempts
		   SET outcome = ?,
		       failure_reason = ?,
		       evidence_json = ?,
		       dom_verified = ?,
		       status = ?,
		       finished_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		string(outcome), failureReason, string(evidenceJSON), domVerified, terminalStatus, attemptID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AdvanceAttemptStatus moves the attempt through its lifecycle (queued
// → composer_opened → typed → submitted → verifying). Terminal statuses
// (dom_verified, failed) are written by FinishExecutionAttempt, not here.
// Best-effort: caller does NOT need to call this before FinishExecutionAttempt;
// it's purely instrumentation for the click pipeline.
func (s *Store) AdvanceAttemptStatus(ctx context.Context, attemptID int64, status models.AttemptStatus) error {
	if attemptID <= 0 {
		return fmt.Errorf("execution_attempts: attempt_id required")
	}
	// tenant-ok: see FinishExecutionAttempt rationale — attempt_id is
	// the issuance token and never exposed to other tenants.
	_, err := s.db.ExecContext(ctx,
		`UPDATE execution_attempts SET status = ? WHERE id = ?`,
		string(status), attemptID,
	)
	return err
}

// GetExecutionAttempt loads a single row by id. Used by tests and by
// observers that want to read the latest evidence for an attempt.
// tenant-ok: attemptID is an internal autoincrement issued in an
// org-scoped tx — see FinishExecutionAttempt rationale.
func (s *Store) GetExecutionAttempt(ctx context.Context, attemptID int64) (models.ExecutionAttempt, error) {
	var a models.ExecutionAttempt
	var startedAt string
	var finishedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, action_ledger_id, outbound_id, org_id, account_id, target_url,
		        action_type, attempt, status, outcome, failure_reason, evidence_json,
		        dom_verified, network_verified, started_at, finished_at
		   FROM execution_attempts WHERE id = ?`,
		attemptID,
	).Scan(
		&a.ID, &a.ActionLedgerID, &a.OutboundID, &a.OrgID, &a.AccountID, &a.TargetURL,
		&a.ActionType, &a.Attempt, &a.Status, &a.Outcome, &a.FailureReason, &a.EvidenceJSON,
		&a.DOMVerified, &a.NetworkVerified, &startedAt, &finishedAt,
	)
	if err != nil {
		return a, err
	}
	a.StartedAt = dbutil.ParseSQLiteTime(startedAt)
	if finishedAt.Valid && finishedAt.String != "" {
		a.FinishedAt = dbutil.ParseSQLiteTime(finishedAt.String)
	}
	return a, nil
}

// ListAttemptsForOutbound returns every attempt row tied to a given
// outbound_messages.id, most-recent-first. Used by the dashboard
// "Attempt history" panel and by the retry layer to count prior tries.
// tenant-ok: outboundID is issued by QueueOutboundForOrg in a tenant-
// scoped tx; the caller has already authenticated the outbound row.
func (s *Store) ListAttemptsForOutbound(ctx context.Context, outboundID int64) ([]models.ExecutionAttempt, error) {
	if outboundID <= 0 {
		return nil, fmt.Errorf("execution_attempts: outbound_id required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, action_ledger_id, outbound_id, org_id, account_id, target_url,
		        action_type, attempt, status, outcome, failure_reason, evidence_json,
		        dom_verified, network_verified, started_at, finished_at
		   FROM execution_attempts
		  WHERE outbound_id = ?
		  ORDER BY attempt DESC, started_at DESC`,
		outboundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ExecutionAttempt
	for rows.Next() {
		var a models.ExecutionAttempt
		var startedAt string
		var finishedAt sql.NullString
		if err := rows.Scan(
			&a.ID, &a.ActionLedgerID, &a.OutboundID, &a.OrgID, &a.AccountID, &a.TargetURL,
			&a.ActionType, &a.Attempt, &a.Status, &a.Outcome, &a.FailureReason, &a.EvidenceJSON,
			&a.DOMVerified, &a.NetworkVerified, &startedAt, &finishedAt,
		); err != nil {
			return nil, err
		}
		a.StartedAt = dbutil.ParseSQLiteTime(startedAt)
		if finishedAt.Valid && finishedAt.String != "" {
			a.FinishedAt = dbutil.ParseSQLiteTime(finishedAt.String)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountRecentAttemptsByAccount returns how many attempts an account has
// made in the given window, optionally bounded by outcome. Used by the
// retry policy + future orchestrator to decide whether to keep trying.
// since=zero means "no time bound" (returns lifetime count for the account).
func (s *Store) CountRecentAttemptsByAccount(ctx context.Context, orgID, accountID int64, outcome models.ExecutionOutcome, since time.Time) (int, error) {
	if orgID <= 0 {
		return 0, fmt.Errorf("execution_attempts: org_id required")
	}
	q := `SELECT COUNT(*) FROM execution_attempts WHERE org_id = ? AND account_id = ?`
	args := []any{orgID, accountID}
	if strings.TrimSpace(string(outcome)) != "" {
		q += ` AND outcome = ?`
		args = append(args, string(outcome))
	}
	if !since.IsZero() {
		q += ` AND started_at >= ?`
		args = append(args, since.UTC().Format("2006-01-02 15:04:05"))
	}
	var n int
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ── Step 4a — Observability queries ──────────────────────────────────────────
// Pure read surfaces backing the dashboard's Execution Reality view. No
// auto-decisions, no scoring, no orchestration — just SELECT and project.
// All queries are org-scoped; callers MUST supply orgID. Time windows are
// inclusive of `since`.

// OutcomeDistributionBucket is one cell of the outcome × action_type grid
// the dashboard renders as a stacked bar / matrix.
type OutcomeDistributionBucket struct {
	Outcome    string `json:"outcome"`
	ActionType string `json:"action_type"`
	Count      int    `json:"count"`
}

// ExecutionOutcomeDistribution returns counts grouped by (outcome,
// action_type) for the given org since `since`. Includes only rows with a
// classified outcome (excludes still-pending attempts). The dashboard
// reads this to answer "what fraction of the last 24h was dom_verified
// vs shadow_rejected vs rate_limited?" without scanning individual rows.
func (s *Store) ExecutionOutcomeDistribution(ctx context.Context, orgID int64, since time.Time) ([]OutcomeDistributionBucket, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("execution_attempts: org_id required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT outcome, action_type, COUNT(*) AS n
		   FROM execution_attempts
		  WHERE org_id = ?
		    AND started_at >= ?
		    AND outcome != ''
		  GROUP BY outcome, action_type
		  ORDER BY n DESC`,
		orgID, since.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutcomeDistributionBucket
	for rows.Next() {
		var b OutcomeDistributionBucket
		if err := rows.Scan(&b.Outcome, &b.ActionType, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListRecentExecutionAttempts returns the most-recent N attempts for an
// org, newest first. Includes the JSON evidence so the dashboard can
// surface comment permalinks / message bubble ids without a second
// fetch. Bounded by `limit` (default 100, max 500) — this is a human-
// observation table, not a paginated list.
func (s *Store) ListRecentExecutionAttempts(ctx context.Context, orgID int64, since time.Time, limit int) ([]models.ExecutionAttempt, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("execution_attempts: org_id required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	// SQLite CURRENT_TIMESTAMP has second-level resolution; multiple attempts
	// inserted in the same second would tie on started_at and the dashboard
	// would show arbitrary order. Tiebreak on id DESC so insertion order is
	// preserved (id is the AUTOINCREMENT primary key — monotonic by definition).
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, action_ledger_id, outbound_id, org_id, account_id, target_url,
		        action_type, attempt, status, outcome, failure_reason, evidence_json,
		        dom_verified, network_verified, started_at, finished_at
		   FROM execution_attempts
		  WHERE org_id = ? AND started_at >= ?
		  ORDER BY started_at DESC, id DESC
		  LIMIT ?`,
		orgID, since.UTC().Format("2006-01-02 15:04:05"), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ExecutionAttempt
	for rows.Next() {
		var a models.ExecutionAttempt
		var startedAt string
		var finishedAt sql.NullString
		if err := rows.Scan(
			&a.ID, &a.ActionLedgerID, &a.OutboundID, &a.OrgID, &a.AccountID, &a.TargetURL,
			&a.ActionType, &a.Attempt, &a.Status, &a.Outcome, &a.FailureReason, &a.EvidenceJSON,
			&a.DOMVerified, &a.NetworkVerified, &startedAt, &finishedAt,
		); err != nil {
			return nil, err
		}
		a.StartedAt = dbutil.ParseSQLiteTime(startedAt)
		if finishedAt.Valid && finishedAt.String != "" {
			a.FinishedAt = dbutil.ParseSQLiteTime(finishedAt.String)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AccountHealthRow is one account's view as the dashboard renders it:
// trust preset + runtime risk numbers + cooldown state. The dashboard
// pairs this with ExecutionOutcomeDistribution to show "this account is
// healthy / poisoned" at a glance.
type AccountHealthRow struct {
	AccountID      int64     `json:"account_id"`
	TrustLevel     string    `json:"trust_level"`
	RiskScore      float64   `json:"risk_score"`
	RecentFailures int       `json:"recent_failures"`
	CooldownUntil  time.Time `json:"cooldown_until"`
	LastActionAt   time.Time `json:"last_action_at"`
	CommentsToday  int       `json:"comments_today"`
	InboxToday     int       `json:"inbox_today"`
}

// AccountHealthSnapshot returns the live behaviour-profile state for
// every account in the org (or one specific account when accountID > 0).
// LEFT JOIN means accounts without a profile row still appear with
// trust_level="" — the dashboard renders that as "default warming."
// Order: highest risk first so poisoned accounts surface immediately.
func (s *Store) AccountHealthSnapshot(ctx context.Context, orgID, accountID int64) ([]AccountHealthRow, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("account_health: org_id required")
	}
	q := `SELECT rs.account_id, COALESCE(p.trust_level, '') AS trust_level,
	             rs.risk_score, rs.recent_failures,
	             COALESCE(rs.cooldown_until, '') AS cooldown_until,
	             COALESCE(rs.last_action_at, '') AS last_action_at,
	             rs.comments_today, rs.inbox_today
	      FROM account_runtime_state rs
	      LEFT JOIN account_behaviour_profiles p ON p.account_id = rs.account_id
	      WHERE rs.org_id = ?`
	args := []any{orgID}
	if accountID > 0 {
		q += ` AND rs.account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY rs.risk_score DESC, rs.recent_failures DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountHealthRow
	for rows.Next() {
		var r AccountHealthRow
		var cooldownStr, lastActionStr string
		if err := rows.Scan(&r.AccountID, &r.TrustLevel, &r.RiskScore, &r.RecentFailures,
			&cooldownStr, &lastActionStr, &r.CommentsToday, &r.InboxToday); err != nil {
			return nil, err
		}
		if cooldownStr != "" {
			r.CooldownUntil = dbutil.ParseSQLiteTime(cooldownStr)
		}
		if lastActionStr != "" {
			r.LastActionAt = dbutil.ParseSQLiteTime(lastActionStr)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
