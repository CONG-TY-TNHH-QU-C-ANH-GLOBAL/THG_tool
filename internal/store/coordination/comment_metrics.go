package coordination

import (
	"context"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// Comment outcome metrics (spec: specs/COMMENT_ASYNC_REVERIFY.md companion, Part C). A
// read-only summary across outbound_messages (verification_outcome buckets) + action_ledger
// (correction reasons) + comment_reverify (errors). tenant-ok: cross-domain reads.

// CommentOutcomeMetrics tallies comment outcomes for an org since `since`.
func (s *Store) CommentOutcomeMetrics(ctx context.Context, orgID int64, since time.Time) (models.CommentMetrics, error) {
	var m models.CommentMetrics
	if orgID <= 0 {
		return m, nil
	}
	sinceStr := since.UTC().Format("2006-01-02 15:04:05")

	// 1) verification_outcome buckets. tenant-ok: cross-domain read (coordination -> outbound).
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(verification_outcome,''), COUNT(*)
		   FROM outbound_messages
		  WHERE org_id = ? AND type = 'comment' AND created_at >= ?
		  GROUP BY verification_outcome`,
		orgID, sinceStr)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var vo string
		var n int
		if err := rows.Scan(&vo, &n); err != nil {
			return m, err
		}
		m.Total += n
		switch strings.TrimSpace(vo) {
		case string(models.VerifVerifiedSuccess):
			m.VerifiedSuccess += n
		case string(models.VerifSubmittedUnverified):
			m.SubmittedUnverified += n
		case string(models.VerifTargetNotReached):
			m.TargetNotReached += n
		case string(models.VerifExecutionFailed):
			m.ExecutionFailed += n
		case "":
			// planned/executing — not a terminal outcome; ignore in failure buckets.
		default:
			m.OtherFailed += n
		}
	}
	if err := rows.Err(); err != nil {
		return m, err
	}

	// 2) corrections + the comment_button_not_found subset (action_ledger.reason).
	m.HumanVerified = s.countLedgerReason(ctx, orgID, "succeeded", models.LedgerReasonHumanVerified, sinceStr)
	m.Reverified = s.countLedgerReason(ctx, orgID, "succeeded", ReverifyCorrectionReason, sinceStr)
	m.CommentButtonNotFound = s.countLedgerReason(ctx, orgID, "", "comment_button_not_found", sinceStr)

	// 3) reverify errors.
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM comment_reverify WHERE org_id = ? AND outcome = 'error' AND created_at >= ?`,
		orgID, sinceStr).Scan(&m.ReverifyError)

	return m, nil
}

// countLedgerReason counts comment ledger rows with the given reason (and outcome, if set).
func (s *Store) countLedgerReason(ctx context.Context, orgID int64, outcome, reason, sinceStr string) int {
	q := `SELECT COUNT(*) FROM action_ledger
	       WHERE org_id = ? AND action_type = 'comment' AND reason = ? AND performed_at >= ?`
	args := []any{orgID, reason, sinceStr}
	if outcome != "" {
		q += ` AND outcome = ?`
		args = append(args, outcome)
	}
	var n int
	_ = s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n
}
