package coordination

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Apply a reverify result (spec: specs/COMMENT_ASYNC_REVERIFY.md, PR-A). This is the
// testable core the result endpoint wraps: on a positive match it appends the append-only
// correction; on a miss it records not_found. Idempotent — a row that already resolved is
// left untouched.

// ApplyReverifyResult records the extension's reverify verdict for one queue row. Returns
// whether a 'succeeded' correction was appended. found=true → append correction +
// mark verified; found=false → mark not_found (the lead stays submitted_unverified until
// the verification cooldown lets it retry).
func (s *Store) ApplyReverifyResult(ctx context.Context, orgID, id int64, found bool, permalink, notes string) (bool, error) {
	if orgID <= 0 || id <= 0 {
		return false, fmt.Errorf("reverify result requires org_id + id")
	}
	job, outcome, err := s.getReverify(ctx, id)
	if err != nil {
		return false, err
	}
	if job.OrgID != orgID {
		return false, fmt.Errorf("reverify %d not in org %d", id, orgID) // tenant guard
	}
	if outcome != ReverifyPending {
		return false, nil // already resolved — idempotent no-op
	}
	if found {
		if _, err := s.AppendReverifyCorrection(ctx, job); err != nil {
			return false, err
		}
		reason := "found"
		if p := strings.TrimSpace(permalink); p != "" {
			reason = "found:" + p
		}
		return true, s.RecordReverifyOutcome(ctx, id, ReverifyVerified, reason)
	}
	return false, s.RecordReverifyOutcome(ctx, id, ReverifyNotFound, strings.TrimSpace(notes))
}

// getReverify loads a reverify row's job fields + current outcome.
func (s *Store) getReverify(ctx context.Context, id int64) (ReverifyJob, string, error) {
	var (
		j       ReverifyJob
		outcome string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, org_id, outbound_id, COALESCE(target_url,''), account_id,
		        COALESCE(created_by,0), COALESCE(content,''), COALESCE(outcome,'')
		   FROM comment_reverify WHERE id = ?`,
		id,
	).Scan(&j.ID, &j.OrgID, &j.OutboundID, &j.TargetURL, &j.AccountID, &j.CreatedBy, &j.Content, &outcome)
	if err == sql.ErrNoRows {
		return j, "", fmt.Errorf("reverify %d not found", id)
	}
	if err != nil {
		return j, "", err
	}
	return j, outcome, nil
}
