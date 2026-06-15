package coordination

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CAS + lease transitions for direct_post_comment_workflows (PR-1 data foundation).
// Every write is org-scoped and uses compare-and-set on the expected prior status so
// a stale poller cannot clobber a newer state. No secrets ever land in error_message.

func dpwTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

// dpwClaimableStatuses are the non-terminal, runnable states the poller advances.
var dpwClaimableStatuses = []string{
	DPStatusRequested, DPStatusImportQueued, DPStatusImporting,
	DPStatusLeadCreated, DPStatusRetryScheduled,
}

// ClaimDueDirectPostCommentWorkflows atomically leases up to `limit` due workflows to
// workerID until leaseUntil, and returns them. "Due" = a claimable status, next_run_at
// elapsed (or null), and no live lease. SQLite serializes writers, so two pollers can
// never double-claim: the loser's UPDATE sees the now-future lease_until and skips.
// No RETURNING (SQLite-safe) — the claimed rows are re-read by (lease_owner, lease_until).
func (s *Store) ClaimDueDirectPostCommentWorkflows(ctx context.Context, now time.Time, workerID string, leaseUntil time.Time, limit int) ([]*DirectPostCommentWorkflow, error) {
	if strings.TrimSpace(workerID) == "" || limit <= 0 {
		return nil, fmt.Errorf("claim requires workerID and positive limit")
	}
	nowS, leaseS := dpwTime(now), dpwTime(leaseUntil)
	placeholders := strings.TrimRight(strings.Repeat("?,", len(dpwClaimableStatuses)), ",")
	args := []any{workerID, leaseS, nowS, nowS}
	for _, st := range dpwClaimableStatuses {
		args = append(args, st)
	}
	args = append(args, nowS, nowS, limit)
	if _, err := s.db.ExecContext(ctx,
		`UPDATE direct_post_comment_workflows
		 SET lease_owner = ?, lease_until = ?, last_attempt_at = ?, updated_at = ?
		 WHERE id IN (
			 SELECT id FROM direct_post_comment_workflows
			 WHERE status IN (`+placeholders+`)
			   AND (next_run_at IS NULL OR next_run_at <= ?)
			   AND (lease_until IS NULL OR lease_until <= ?)
			 ORDER BY next_run_at LIMIT ?
		 )`, args...); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+dpwColumns+` FROM direct_post_comment_workflows
		 WHERE lease_owner = ? AND lease_until = ? ORDER BY id`, workerID, leaseS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DirectPostCommentWorkflow
	for rows.Next() {
		w, err := scanDPW(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// casOK runs an org-scoped UPDATE and reports whether exactly the expected row moved
// (rows-affected > 0). The caller composes the SET/WHERE; this centralizes the
// rows-affected check so a no-op (stale prior status) is a clean false, not an error.
func (s *Store) casOK(ctx context.Context, query string, args ...any) (bool, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// MarkDirectPostImportQueued: requested/retry_scheduled → import_queued (records the
// single-post import task id). Idempotent via CAS on the prior status.
func (s *Store) MarkDirectPostImportQueued(ctx context.Context, orgID, id int64, importTaskID string) (bool, error) {
	return s.casOK(ctx,
		`UPDATE direct_post_comment_workflows
		 SET status = ?, import_task_id = ?, error_code = '', error_message = '',
		     lease_owner = '', lease_until = NULL, updated_at = ?
		 WHERE org_id = ? AND id = ? AND status IN (?, ?)`,
		DPStatusImportQueued, strings.TrimSpace(importTaskID), dpwTime(time.Now()),
		orgID, id, DPStatusRequested, DPStatusRetryScheduled)
}

// MarkDirectPostLeadCreated: import_queued/importing/retry_scheduled → lead_created
// (records lead_id). retry_scheduled is accepted because the poller claims a
// retry-scheduled workflow whose post lead has since appeared and advances it.
func (s *Store) MarkDirectPostLeadCreated(ctx context.Context, orgID, id, leadID int64) (bool, error) {
	if leadID <= 0 {
		return false, fmt.Errorf("lead_created requires lead_id")
	}
	return s.casOK(ctx,
		`UPDATE direct_post_comment_workflows
		 SET status = ?, lead_id = ?, lease_owner = '', lease_until = NULL, updated_at = ?
		 WHERE org_id = ? AND id = ? AND status IN (?, ?, ?)`,
		DPStatusLeadCreated, leadID, dpwTime(time.Now()),
		orgID, id, DPStatusImportQueued, DPStatusImporting, DPStatusRetryScheduled)
}

// MarkDirectPostCommentQueued: lead_created → comment_queued. No outbound id is stored
// here — the outbound row is OWNED by the outbound domain and resolved via lead_id;
// duplicating its id in this table would invert the ownership boundary.
func (s *Store) MarkDirectPostCommentQueued(ctx context.Context, orgID, id int64) (bool, error) {
	return s.casOK(ctx,
		`UPDATE direct_post_comment_workflows
		 SET status = ?, lease_owner = '', lease_until = NULL, updated_at = ?
		 WHERE org_id = ? AND id = ? AND status = ?`,
		DPStatusCommentQueued, dpwTime(time.Now()), orgID, id, DPStatusLeadCreated)
}

// MarkDirectPostCompleted: comment_queued → completed.
func (s *Store) MarkDirectPostCompleted(ctx context.Context, orgID, id int64) (bool, error) {
	now := dpwTime(time.Now())
	return s.casOK(ctx,
		`UPDATE direct_post_comment_workflows
		 SET status = ?, completed_at = ?, lease_owner = '', lease_until = NULL, updated_at = ?
		 WHERE org_id = ? AND id = ? AND status = ?`,
		DPStatusCompleted, now, now, orgID, id, DPStatusCommentQueued)
}

// MarkDirectPostFailed terminally fails a non-terminal workflow with a typed reason.
// errorMessage MUST NOT contain secrets (cookies/tokens/session) — callers pass a
// short typed reason; the column is for operator drill-down only.
func (s *Store) MarkDirectPostFailed(ctx context.Context, orgID, id int64, errorCode, errorMessage string) (bool, error) {
	return s.casOK(ctx,
		`UPDATE direct_post_comment_workflows
		 SET status = ?, error_code = ?, error_message = ?, lease_owner = '',
		     lease_until = NULL, completed_at = ?, updated_at = ?
		 WHERE org_id = ? AND id = ? AND status NOT IN ('completed','failed','cancelled')`,
		DPStatusFailed, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage),
		dpwTime(time.Now()), dpwTime(time.Now()), orgID, id)
}

// ScheduleDirectPostRetry re-queues a non-terminal workflow for a later attempt:
// status → retry_scheduled, next_run_at = nextRunAt, retry_count++, lease released.
// errorCode carries the actionable reason (e.g. login_required) for observability.
func (s *Store) ScheduleDirectPostRetry(ctx context.Context, orgID, id int64, nextRunAt time.Time, errorCode, errorMessage string) (bool, error) {
	return s.casOK(ctx,
		`UPDATE direct_post_comment_workflows
		 SET status = ?, next_run_at = ?, retry_count = retry_count + 1,
		     error_code = ?, error_message = ?, lease_owner = '', lease_until = NULL, updated_at = ?
		 WHERE org_id = ? AND id = ? AND status NOT IN ('completed','failed','cancelled')`,
		DPStatusRetryScheduled, dpwTime(nextRunAt), strings.TrimSpace(errorCode),
		strings.TrimSpace(errorMessage), dpwTime(time.Now()), orgID, id)
}
