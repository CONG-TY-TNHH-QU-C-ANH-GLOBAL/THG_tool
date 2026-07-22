package crawlrun_test

import (
	"context"
	"sync"
	"testing"

	"github.com/thg/scraper/internal/store/crawlrun"
)

// TestApplyRunResult_TransactionErrorRollsBackEverything forces a
// mid-transaction failure via a test-only trigger on the magic hash, AFTER an
// earlier candidate in the same batch was inserted, proving all-or-nothing.
func TestApplyRunResult_TransactionErrorRollsBackEverything(t *testing.T) {
	st, db := open(t)
	const org = 45021
	cleanupOrg(t, db, org)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION thg_test_apply_boom() RETURNS trigger AS $$
		BEGIN
			IF NEW.post_dedup_hash = 'boom-45021' THEN
				RAISE EXCEPTION 'thg_test_apply_boom';
			END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql`); err != nil {
		t.Fatalf("create boom function: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER trg_thg_test_apply_boom
		BEFORE INSERT ON facebook_crawl_lead_index
		FOR EACH ROW EXECUTE FUNCTION thg_test_apply_boom()`); err != nil {
		t.Fatalf("create boom trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DROP TRIGGER IF EXISTS trg_thg_test_apply_boom ON facebook_crawl_lead_index`)
		_, _ = db.ExecContext(context.Background(), `DROP FUNCTION IF EXISTS thg_test_apply_boom()`)
	})

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	in := resultInput(run, org, "h-first", "boom-45021")
	if _, err := st.ApplyRunResult(ctx, in); err == nil {
		t.Fatalf("forced constraint failure must surface as an error")
	}

	// All-or-nothing: the earlier candidate, the run transition, and the source
	// stamp must all have rolled back.
	if r := readRun(t, db, org, run.RunID); r.status != "running" || r.finishedAt.Valid {
		t.Fatalf("run mutated despite rollback: %+v", r)
	}
	if got := indexRows(t, db, org); len(got) != 0 {
		t.Fatalf("partial batch survived rollback: %v", got)
	}
	var lastRunSet bool
	if err := db.QueryRowContext(ctx,
		`SELECT last_run_at IS NOT NULL FROM facebook_crawl_campaign_sources
		 WHERE org_id = $1 AND id = $2`, org, run.SourceID).Scan(&lastRunSet); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if lastRunSet {
		t.Fatalf("source stamp survived rollback")
	}

	// The run is still applicable: a clean retry of the apply succeeds.
	mustApply(t, st, resultInput(run, org, "h-first"), crawlrun.ApplyApplied)
}

func TestApplyRunResult_ConcurrentSameRunSingleWinner(t *testing.T) {
	st, db := open(t)
	const org = 45022
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	const workers = 8
	in := resultInput(run, org, "h-a", "h-b")
	results := make([]crawlrun.ApplyResult, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := st.ApplyRunResult(context.Background(), in)
			results[i], errs[i] = out.Result, err
		}(i)
	}
	wg.Wait()

	applied, replayed := 0, 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		switch results[i] {
		case crawlrun.ApplyApplied:
			applied++
		case crawlrun.ApplyAlreadyApplied:
			replayed++
		default:
			t.Fatalf("worker %d unexpected result %q", i, results[i])
		}
	}
	if applied != 1 || replayed != workers-1 {
		t.Fatalf("want exactly one winner: applied=%d replayed=%d", applied, replayed)
	}
	if got := indexRows(t, db, org); len(got) != 2 {
		t.Fatalf("duplicate index rows under concurrency: %v", got)
	}
}

func TestApplyRunResult_ConcurrentOverlappingRunsNoDuplicateIndex(t *testing.T) {
	st, db := open(t)
	const org = 45023
	cleanupOrg(t, db, org)

	runA := runningRun(t, st, db, seedCampaign(t, db, org, 240, 1440), fixedNow)
	runB := runningRun(t, st, db, seedCampaign(t, db, org, 240, 1440), fixedNow)

	var wg sync.WaitGroup
	outs := make([]crawlrun.ApplyRunResultOutcome, 2)
	errs := make([]error, 2)
	for i, run := range []crawlrun.ClaimedRun{runA, runB} {
		wg.Add(1)
		go func(i int, run crawlrun.ClaimedRun) {
			defer wg.Done()
			outs[i], errs[i] = st.ApplyRunResult(context.Background(),
				resultInput(run, org, "h-contested", "h-own-"+string(rune('a'+i))))
		}(i, run)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if outs[i].Result != crawlrun.ApplyApplied {
			t.Fatalf("run %d result %q", i, outs[i].Result)
		}
	}
	if outs[0].LeadsIndexed+outs[1].LeadsIndexed != 3 {
		t.Fatalf("contested identity double-indexed: %+v %+v", outs[0], outs[1])
	}
	idx := indexRows(t, db, org)
	if len(idx) != 3 {
		t.Fatalf("index rows = %v, want 3", idx)
	}
}
