package crawlrun_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/store/crawlrun"
)

func recoverInput(org int64, run crawlrun.ClaimedRun) crawlrun.RecoverDispatchFailureInput {
	return crawlrun.RecoverDispatchFailureInput{
		Fence:             crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt},
		ExpectedAccountID: run.AccountID,
		Now:               fixedNow,
	}
}

func TestRecoverDispatchFailure_FailsParentAndQueuesRetry(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 44001
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	out, err := st.RecoverDispatchFailure(ctx, recoverInput(org, run))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if out.Result != crawlrun.RecoverRecovered {
		t.Fatalf("result = %q, want recovered", out.Result)
	}
	if parentStatus, _ := runStatus(t, db, org, run.RunID); parentStatus != "failed" {
		t.Fatalf("parent status = %q, want failed", parentStatus)
	}
	retryStatus, retryAccount := runStatus(t, db, org, out.RetryRunID)
	if retryStatus != "queued" || retryAccount.Valid {
		t.Fatalf("retry status=%q account=%v, want queued/NULL", retryStatus, retryAccount)
	}
	var attempt int
	if err := db.QueryRowContext(ctx,
		`SELECT attempt FROM facebook_crawl_runs WHERE org_id=$1 AND id=$2`, org, out.RetryRunID).Scan(&attempt); err != nil {
		t.Fatalf("read retry attempt: %v", err)
	}
	if attempt != run.Attempt+1 {
		t.Fatalf("retry attempt = %d, want %d", attempt, run.Attempt+1)
	}
}

func TestRecoverDispatchFailure_IdempotentRepeat(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 44002
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	first, err := st.RecoverDispatchFailure(ctx, recoverInput(org, run))
	if err != nil {
		t.Fatalf("first recover: %v", err)
	}
	second, err := st.RecoverDispatchFailure(ctx, recoverInput(org, run))
	if err != nil {
		t.Fatalf("second recover: %v", err)
	}
	if second.Result != crawlrun.RecoverAlreadyRecovered {
		t.Fatalf("repeat result = %q, want already_recovered", second.Result)
	}
	if second.RetryRunID != first.RetryRunID {
		t.Fatalf("repeat reused retry %d, want the original %d", second.RetryRunID, first.RetryRunID)
	}
}

func TestRecoverDispatchFailure_StaleFenceNoOp(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 44003
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	in := recoverInput(org, run)
	in.Fence.Attempt = run.Attempt + 1 // a newer attempt the caller doesn't own
	out, err := st.RecoverDispatchFailure(ctx, in)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if out.Result != crawlrun.RecoverStaleAttempt {
		t.Fatalf("result = %q, want stale_attempt", out.Result)
	}
	if status, _ := runStatus(t, db, org, run.RunID); status != "running" {
		t.Fatalf("stale recovery must not touch the run; status = %q", status)
	}
}

func TestRecoverDispatchFailure_WrongAccountIsStale(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 44005
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	in := recoverInput(org, run)
	in.ExpectedAccountID = run.AccountID + 99999 // a valid but wrong account id
	out, err := st.RecoverDispatchFailure(ctx, in)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if out.Result != crawlrun.RecoverStaleAttempt {
		t.Fatalf("result = %q, want stale_attempt", out.Result)
	}
	if status, _ := runStatus(t, db, org, run.RunID); status != "running" {
		t.Fatalf("wrong-account recovery must leave the run running; status = %q", status)
	}
	var retries int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM facebook_crawl_runs WHERE org_id=$1 AND retry_of_run_id IS NOT NULL`,
		org).Scan(&retries); err != nil {
		t.Fatalf("count retries: %v", err)
	}
	if retries != 0 {
		t.Fatalf("wrong-account recovery created %d retry rows, want 0", retries)
	}
}

func TestRecoverDispatchFailure_ParentTerminalOtherReason(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 44004
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)
	// The run already finished for an unrelated reason before recovery ran.
	if _, err := db.ExecContext(ctx,
		`UPDATE facebook_crawl_runs SET status='succeeded', finished_at=$3 WHERE org_id=$1 AND id=$2`,
		org, run.RunID, fixedNow); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}

	out, err := st.RecoverDispatchFailure(ctx, recoverInput(org, run))
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if out.Result != crawlrun.RecoverParentNotRunning {
		t.Fatalf("result = %q, want parent_not_running", out.Result)
	}
	if status, _ := runStatus(t, db, org, run.RunID); status != "succeeded" {
		t.Fatalf("recovery must not overwrite a terminal run; status = %q", status)
	}
}
