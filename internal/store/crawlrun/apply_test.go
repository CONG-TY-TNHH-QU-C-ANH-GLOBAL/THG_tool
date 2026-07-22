package crawlrun_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/crawlrun"
)

// resultInput builds a valid successful-run input for the claimed run.
func resultInput(run crawlrun.ClaimedRun, org int64, hashes ...string) crawlrun.ApplyRunResultInput {
	leads := make([]crawlrun.LeadCandidate, 0, len(hashes))
	for _, h := range hashes {
		leads = append(leads, crawlrun.LeadCandidate{PostDedupHash: h})
	}
	return crawlrun.ApplyRunResultInput{
		Fence:          crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt},
		Status:         crawlrun.TerminalSucceeded,
		ExitReasonCode: "frontier_reached",
		Counters:       crawlrun.RunCounters{PostsSeen: 5, FreshLeadCount: len(hashes), StaleSkipped: 1},
		Leads:          leads,
		NewestPostAt:   fixedNow.Add(-30 * time.Minute),
		Now:            fixedNow.Add(10 * time.Minute),
	}
}

func mustApply(t *testing.T, st *crawlrun.Store, in crawlrun.ApplyRunResultInput,
	want crawlrun.ApplyResult) crawlrun.ApplyRunResultOutcome {
	t.Helper()
	out, err := st.ApplyRunResult(context.Background(), in)
	if err != nil {
		t.Fatalf("ApplyRunResult: %v", err)
	}
	if out.Result != want {
		t.Fatalf("ApplyRunResult result = %q, want %q", out.Result, want)
	}
	return out
}

type runRow struct {
	status         string
	exitReasonCode string
	postsSeen      int
	freshLeadCount int
	accountID      sql.NullInt64
	finishedAt     sql.NullTime
}

func readRun(t *testing.T, db *sql.DB, org, runID int64) runRow {
	t.Helper()
	var r runRow
	if err := db.QueryRowContext(context.Background(),
		`SELECT status, exit_reason_code, posts_seen, fresh_lead_count, account_id, finished_at
		 FROM facebook_crawl_runs WHERE org_id = $1 AND id = $2`,
		org, runID).Scan(&r.status, &r.exitReasonCode, &r.postsSeen, &r.freshLeadCount,
		&r.accountID, &r.finishedAt); err != nil {
		t.Fatalf("read run: %v", err)
	}
	return r
}

func indexRows(t *testing.T, db *sql.DB, org int64) map[string]int64 {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT post_dedup_hash, run_id FROM facebook_crawl_lead_index WHERE org_id = $1`, org)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	defer rows.Close()
	got := map[string]int64{}
	for rows.Next() {
		var hash string
		var runID int64
		if err := rows.Scan(&hash, &runID); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		got[hash] = runID
	}
	return got
}

func TestApplyRunResult_AtomicSuccess(t *testing.T) {
	st, db := open(t)
	const org = 45001
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	in := resultInput(run, org, "h-a", "h-b")
	out := mustApply(t, st, in, crawlrun.ApplyApplied)
	if out.CandidatesReceived != 2 || out.LeadsIndexed != 2 || out.LeadsAlreadyKnown != 0 || out.DuplicatesInBatch != 0 {
		t.Fatalf("counts = %+v", out)
	}

	r := readRun(t, db, org, run.RunID)
	if r.status != "succeeded" || r.exitReasonCode != "frontier_reached" ||
		r.postsSeen != 5 || r.freshLeadCount != 2 || !r.finishedAt.Valid {
		t.Fatalf("terminal row = %+v", r)
	}
	if !r.accountID.Valid || r.accountID.Int64 != s.account {
		t.Fatalf("account ownership not preserved: %+v", r.accountID)
	}

	idx := indexRows(t, db, org)
	if len(idx) != 2 || idx["h-a"] != run.RunID || idx["h-b"] != run.RunID {
		t.Fatalf("index rows = %v", idx)
	}
	var leadID sql.NullInt64
	if err := db.QueryRowContext(context.Background(),
		`SELECT lead_id FROM facebook_crawl_lead_index WHERE org_id = $1 AND post_dedup_hash = 'h-a'`,
		org).Scan(&leadID); err != nil || leadID.Valid {
		t.Fatalf("lead_id must stay NULL until PR-M5 (err=%v valid=%v)", err, leadID.Valid)
	}

	var lastRun, cursor sql.NullTime
	if err := db.QueryRowContext(context.Background(),
		`SELECT last_run_at, cursor_last_post_at FROM facebook_crawl_campaign_sources
		 WHERE org_id = $1 AND id = $2`, org, run.SourceID).Scan(&lastRun, &cursor); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !lastRun.Valid || !lastRun.Time.Equal(in.Now) {
		t.Fatalf("last_run_at = %v, want %v", lastRun, in.Now)
	}
	if !cursor.Valid || !cursor.Time.Equal(in.NewestPostAt) {
		t.Fatalf("cursor_last_post_at = %v, want %v", cursor, in.NewestPostAt)
	}
}

func TestApplyRunResult_ExactReplayIsDeterministic(t *testing.T) {
	st, db := open(t)
	const org = 45002
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	in := resultInput(run, org, "h-a", "h-b")
	first := mustApply(t, st, in, crawlrun.ApplyApplied)
	replay := mustApply(t, st, in, crawlrun.ApplyAlreadyApplied)
	if replay.LeadsIndexed != first.LeadsIndexed || replay.LeadsAlreadyKnown != first.LeadsAlreadyKnown {
		t.Fatalf("replay counts %+v != first %+v", replay, first)
	}
	if len(indexRows(t, db, org)) != 2 {
		t.Fatalf("replay must not create index rows")
	}
}

func TestApplyRunResult_ConflictingReplayRejected(t *testing.T) {
	st, db := open(t)
	const org = 45003
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	mustApply(t, st, resultInput(run, org, "h-a"), crawlrun.ApplyApplied)

	conflicting := resultInput(run, org, "h-a")
	conflicting.Status = crawlrun.TerminalFailed
	conflicting.ExitReasonCode = "timestamp_parser_degraded"
	mustApply(t, st, conflicting, crawlrun.ApplyConflictingReplay)

	if r := readRun(t, db, org, run.RunID); r.status != "succeeded" {
		t.Fatalf("terminal state regressed to %q", r.status)
	}
}

func TestApplyRunResult_BatchDuplicatesAndOverlap(t *testing.T) {
	st, db := open(t)
	const org = 45004
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	// h-known is already reserved by an earlier run in this org.
	prior := runningRun(t, st, db, seedCampaign(t, db, org, 240, 1440), fixedNow)
	mustApply(t, st, resultInput(prior, org, "h-known"), crawlrun.ApplyApplied)

	in := resultInput(run, org, "h-new", "h-known", "h-new")
	out := mustApply(t, st, in, crawlrun.ApplyApplied)
	if out.CandidatesReceived != 3 || out.DuplicatesInBatch != 1 ||
		out.LeadsIndexed != 1 || out.LeadsAlreadyKnown != 1 {
		t.Fatalf("counts = %+v", out)
	}
	if idx := indexRows(t, db, org); idx["h-known"] != prior.RunID || idx["h-new"] != run.RunID {
		t.Fatalf("dedup ownership wrong: %v", idx)
	}
}
