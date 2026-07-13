package crawlrun_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/store/crawlrun"
)

// TestClaimNextRun_SkipsLockedCandidate proves FOR UPDATE SKIP LOCKED with two
// real connections deterministically (no goroutine timing): a second connection
// holds a row lock on the sole queued run, so the claim must skip it and report
// no work rather than block; once the lock releases, the run claims.
func TestClaimNextRun_SkipsLockedCandidate(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 42007
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "only", nil)
	if _, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	hold, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holding tx: %v", err)
	}
	var locked int64
	if err := hold.QueryRowContext(ctx,
		`SELECT id FROM facebook_crawl_runs WHERE org_id = $1 AND status = 'queued' FOR UPDATE`,
		org).Scan(&locked); err != nil {
		_ = hold.Rollback()
		t.Fatalf("lock candidate: %v", err)
	}

	_, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow})
	if err != nil {
		_ = hold.Rollback()
		t.Fatalf("claim while locked: %v", err)
	}
	if ok {
		_ = hold.Rollback()
		t.Fatal("SKIP LOCKED must skip the run another connection holds")
	}

	if err := hold.Rollback(); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if _, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow}); err != nil || !ok {
		t.Fatalf("after release, claim must succeed: ok=%v err=%v", ok, err)
	}
}
