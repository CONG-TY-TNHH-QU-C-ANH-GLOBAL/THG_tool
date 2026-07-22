package crawlrun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// appliedRun is the fenced snapshot of the locked run row.
type appliedRun struct {
	sourceID       int64
	status         string
	attempt        int
	exitReasonCode string
	counters       RunCounters
}

func lockRunForApply(ctx context.Context, tx *sql.Tx, orgID, runID int64) (appliedRun, error) {
	var r appliedRun
	err := tx.QueryRowContext(ctx,
		`SELECT source_id, status, attempt, exit_reason_code,
		        posts_seen, fresh_lead_count, stale_skipped, duplicate_count, unparsed_count
		 FROM facebook_crawl_runs
		 WHERE org_id = $1 AND id = $2
		 FOR UPDATE`,
		orgID, runID).Scan(&r.sourceID, &r.status, &r.attempt, &r.exitReasonCode,
		&r.counters.PostsSeen, &r.counters.FreshLeadCount, &r.counters.StaleSkipped,
		&r.counters.DuplicateCount, &r.counters.UnparsedCount)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("crawlrun: apply lock run: %w", err)
	}
	return r, err
}

// classifyApply is the pure fencing/lifecycle decision over the locked row.
// Attempt mismatch always collapses to stale — an older retry can never
// overwrite a newer attempt, and a newer token never resurrects an older row.
func classifyApply(run appliedRun, in ApplyRunResultInput) ApplyResult {
	if run.attempt != in.Fence.Attempt {
		return ApplyStaleRejected
	}
	switch run.status {
	case "running":
		return ApplyApplied
	case string(TerminalSucceeded), string(TerminalStoppedSafe), string(TerminalFailed),
		"abandoned", "cancelled":
		if run.status == string(in.Status) &&
			run.exitReasonCode == in.ExitReasonCode &&
			run.counters == in.Counters {
			return ApplyAlreadyApplied
		}
		return ApplyConflictingReplay
	default:
		return ApplyRunNotRunning
	}
}
