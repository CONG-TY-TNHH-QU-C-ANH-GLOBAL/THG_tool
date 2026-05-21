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

// GapDetectionRow surfaces one outbound row that is stuck — sitting in
// planned/executing for longer than the threshold AND has zero matching
// execution_attempts rows. Either the executor crashed before BeginAttempt
// fired, or the lease holder vanished. Either way the operator needs to
// see the row to decide reset vs cancel.
type GapDetectionRow struct {
	OutboundID     int64     `json:"outbound_id"`
	OrgID          int64     `json:"org_id"`
	AccountID      int64     `json:"account_id"`
	ActionType     string    `json:"action_type"`
	TargetURL      string    `json:"target_url"`
	ExecutionState string    `json:"execution_state"`
	CreatedAt      time.Time `json:"created_at"`
	LeaseExpiry    time.Time `json:"lease_expiry,omitempty"`
	AgeSeconds     int       `json:"age_seconds"`
}

// GapDetection returns outbound rows in {planned, executing} that have
// NO matching execution_attempts row AND were created more than
// olderThan ago. The "no attempt row" predicate is the gap: if the
// executor merely failed verification, an attempts row with a failure
// outcome would exist. A missing row means the executor never even
// reached BeginAttempt — true stuck state. Bounded by `limit`.
func (s *Store) GapDetection(ctx context.Context, orgID int64, olderThan time.Time, limit int) ([]GapDetectionRow, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("gap_detection: org_id required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT om.id, om.org_id, om.account_id, om.type, om.target_url,
		        om.execution_state, om.created_at,
		        COALESCE(om.lease_expiry, '')
		   FROM outbound_messages om
		  WHERE om.org_id = ?
		    AND om.execution_state IN ('planned','executing')
		    AND om.created_at < ?
		    AND NOT EXISTS (SELECT 1 FROM execution_attempts ea WHERE ea.outbound_id = om.id)
		  ORDER BY om.created_at ASC
		  LIMIT ?`,
		orgID, olderThan.UTC().Format("2006-01-02 15:04:05"), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	var out []GapDetectionRow
	for rows.Next() {
		var r GapDetectionRow
		var createdAt, leaseExpiry string
		if err := rows.Scan(&r.OutboundID, &r.OrgID, &r.AccountID, &r.ActionType, &r.TargetURL,
			&r.ExecutionState, &createdAt, &leaseExpiry); err != nil {
			return nil, err
		}
		r.CreatedAt = dbutil.ParseSQLiteTime(createdAt)
		if leaseExpiry != "" {
			r.LeaseExpiry = dbutil.ParseSQLiteTime(leaseExpiry)
		}
		if !r.CreatedAt.IsZero() {
			r.AgeSeconds = int(now.Sub(r.CreatedAt).Seconds())
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TimeseriesBucket is one (hour, outcome) point on the per-account
// outcome timeseries used by the Account Fleet Health drill-down.
// Bucket is RFC3339 hour boundary (e.g. 2026-05-21T14:00:00Z).
type TimeseriesBucket struct {
	Bucket  string `json:"bucket"`
	Outcome string `json:"outcome"`
	Count   int    `json:"count"`
}

// AccountOutcomeTimeseries returns hourly-bucketed outcome counts for
// one account over the requested window. Empty buckets are omitted —
// the dashboard fills the gaps when rendering. Excludes still-pending
// attempts (outcome=''). Per project_runtime_control_plane EXP-3
// scaled down: per-account view, not full Account Fleet table.
func (s *Store) AccountOutcomeTimeseries(ctx context.Context, orgID, accountID int64, since time.Time) ([]TimeseriesBucket, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("account_timeseries: org_id required")
	}
	if accountID <= 0 {
		return nil, fmt.Errorf("account_timeseries: account_id required")
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT strftime('%Y-%m-%dT%H:00:00Z', started_at) AS bucket,
		        outcome,
		        COUNT(*) AS n
		   FROM execution_attempts
		  WHERE org_id = ? AND account_id = ? AND started_at >= ? AND outcome != ''
		  GROUP BY bucket, outcome
		  ORDER BY bucket ASC`,
		orgID, accountID, since.UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimeseriesBucket
	for rows.Next() {
		var b TimeseriesBucket
		if err := rows.Scan(&b.Bucket, &b.Outcome, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ReconcileMismatchRow surfaces an action_ledger row whose outcome
// disagrees with the latest execution_attempt outcome for the same
// outbound_id. Most-common mismatch the dashboard wants to expose:
// ledger says 'succeeded' but the verifier classified the attempt as
// shadow_rejected / blocked / captcha / rate_limited — i.e. the
// ledger is HALLUCINATING success that didn't actually land.
type ReconcileMismatchRow struct {
	LedgerID       int64     `json:"ledger_id"`
	OrgID          int64     `json:"org_id"`
	AccountID      int64     `json:"account_id"`
	OutboundID     int64     `json:"outbound_id"`
	ActionType     string    `json:"action_type"`
	TargetURL      string    `json:"target_url"`
	PerformedAt    time.Time `json:"performed_at"`
	LedgerOutcome  string    `json:"ledger_outcome"`
	AttemptOutcome string    `json:"attempt_outcome"`
}

// LedgerReconcileMismatches surfaces action_ledger rows where the
// ledger.outcome='succeeded' but the LATEST execution_attempts.outcome
// for the same outbound_id is NOT in the success set
// (dom_verified | optimistic_success | duplicate_blocked). These are
// the hallucinated-success rows that corrupt the badge / risk / orchestrator
// downstream consumers warned about in project_execution_verification.
//
// Latest attempt is picked by MAX(id) (autoincrement, monotonic) per
// outbound_id, avoiding correlated subqueries.
func (s *Store) LedgerReconcileMismatches(ctx context.Context, orgID int64, since time.Time, limit int) ([]ReconcileMismatchRow, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("ledger_reconcile: org_id required")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT al.id, al.org_id, al.account_id, al.outbound_id,
		        al.action_type, al.target_url, al.performed_at,
		        al.outcome AS ledger_outcome, ea.outcome AS attempt_outcome
		   FROM action_ledger al
		   JOIN execution_attempts ea
		     ON ea.outbound_id = al.outbound_id
		    AND ea.id = (SELECT MAX(id) FROM execution_attempts WHERE outbound_id = al.outbound_id)
		  WHERE al.org_id = ?
		    AND al.performed_at >= ?
		    AND al.outbound_id > 0
		    AND al.outcome = 'succeeded'
		    AND ea.outcome NOT IN ('dom_verified','optimistic_success','duplicate_blocked','')
		  ORDER BY al.performed_at DESC
		  LIMIT ?`,
		orgID, since.UTC().Format("2006-01-02 15:04:05"), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReconcileMismatchRow
	for rows.Next() {
		var r ReconcileMismatchRow
		var performedAt string
		if err := rows.Scan(&r.LedgerID, &r.OrgID, &r.AccountID, &r.OutboundID,
			&r.ActionType, &r.TargetURL, &performedAt,
			&r.LedgerOutcome, &r.AttemptOutcome); err != nil {
			return nil, err
		}
		r.PerformedAt = dbutil.ParseSQLiteTime(performedAt)
		out = append(out, r)
	}
	return out, rows.Err()
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
