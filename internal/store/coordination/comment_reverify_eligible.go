package coordination

import (
	"context"
	"time"
)

// Reverify eligibility finder (spec: specs/domains/facebook-sales-intelligence/features/comment-automation/technical.md, PR-A). Split from
// comment_reverify.go to keep each file under the size budget.

// FindReverifyEligible returns submitted-unverified comments worth reverifying: finished,
// verification_outcome=submitted_unverified (which by construction means submit reached +
// composer cleared — so failed_before_submit rows like target_not_reached are excluded),
// with a target URL + expected content + actor, older than `olderThan` (the 2–5m delay),
// and not already scheduled. // tenant-ok: cross-domain read (coordination -> outbound).
func (s *Store) FindReverifyEligible(ctx context.Context, olderThan time.Time, limit int) ([]ReverifyJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT om.id, om.org_id, COALESCE(om.target_url,''), om.account_id,
		        COALESCE(om.created_by,0), COALESCE(om.content,'')
		   FROM outbound_messages om
		  WHERE om.type = 'comment'
		    AND om.verification_outcome = ?
		    AND om.execution_state = 'finished'
		    AND TRIM(COALESCE(om.content,'')) != ''
		    AND om.account_id > 0
		    AND TRIM(COALESCE(om.target_url,'')) != ''
		    AND COALESCE(om.sent_at, om.created_at) <= ?
		    AND NOT EXISTS (SELECT 1 FROM comment_reverify cr WHERE cr.outbound_id = om.id)
		  ORDER BY COALESCE(om.sent_at, om.created_at) ASC LIMIT ?`,
		"submitted_unverified", olderThan.UTC().Format("2006-01-02 15:04:05"), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []ReverifyJob
	for rows.Next() {
		var j ReverifyJob
		// The eligible row IS the original outbound; no comment_reverify row exists yet (ID=0).
		if err := rows.Scan(&j.OutboundID, &j.OrgID, &j.TargetURL, &j.AccountID, &j.CreatedBy, &j.Content); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
