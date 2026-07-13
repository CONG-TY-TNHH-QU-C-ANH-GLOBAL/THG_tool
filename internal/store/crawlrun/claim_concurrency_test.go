package crawlrun_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

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

// TestClaimNextRun_ActiveAccountBackstop proves the database is the race
// backstop: an account already running one run cannot claim a second. The store
// must surface the ux_fb_crawl_runs_one_active_account violation, not swallow it
// as no-work.
func TestClaimNextRun_ActiveAccountBackstop(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 42009
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "src-1", nil)
	seedSource(t, db, s, "src-2", nil)
	if _, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	if _, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow}); err != nil || !ok {
		t.Fatalf("first claim must succeed: ok=%v err=%v", ok, err)
	}

	_, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow})
	if ok {
		t.Fatal("a busy account must not report a second claimed run")
	}
	if err == nil {
		t.Fatal("second claim must surface the active-account backstop error, not (zero,false,nil)")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("err = %v, want a pgconn.PgError", err)
	}
	if pgErr.ConstraintName != "ux_fb_crawl_runs_one_active_account" {
		t.Fatalf("constraint = %q, want ux_fb_crawl_runs_one_active_account", pgErr.ConstraintName)
	}
}
