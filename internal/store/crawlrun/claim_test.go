package crawlrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/crawlrun"
)

func TestClaimNextRun_TransitionsToRunning(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 42001
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "src-a", nil)
	if _, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	claimed, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow})
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if claimed.AccountID != s.account || claimed.Attempt != 1 {
		t.Fatalf("claimed = %+v, want account %d attempt 1", claimed, s.account)
	}
	wantCutoff := fixedNow.Add(-1440 * time.Minute)
	if !claimed.FreshCutoffAt.Equal(wantCutoff) {
		t.Fatalf("fresh cutoff = %v, want %v (now - freshness window)", claimed.FreshCutoffAt, wantCutoff)
	}
	status, account := runStatus(t, db, org, claimed.RunID)
	if status != "running" || !account.Valid || account.Int64 != s.account {
		t.Fatalf("row after claim: status=%q account=%v, want running/%d", status, account, s.account)
	}
}

func TestClaimNextRun_NoneClaimable(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 42002
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)

	_, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if ok {
		t.Fatal("no queued run exists; claim must report none")
	}
}

func TestClaimNextRun_AccountNotInPool(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 42003
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "src-a", nil)
	if _, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// An account owned by the org but never added to the campaign pool.
	var outsider int64
	if err := db.QueryRowContext(ctx,
		`INSERT INTO accounts (name, org_id) VALUES ('outsider', $1) RETURNING id`, org).Scan(&outsider); err != nil {
		t.Fatalf("seed outsider: %v", err)
	}

	_, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: outsider, Now: fixedNow})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if ok {
		t.Fatal("account outside the campaign pool must not claim the run")
	}
}

func TestClaimNextRun_TwoAccountsClaimDistinctRuns(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 42004
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	acct2 := addAccount(t, db, s)
	seedSource(t, db, s, "src-1", nil)
	seedSource(t, db, s, "src-2", nil)
	if _, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	first, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: s.account, Now: fixedNow})
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	second, ok, err := st.ClaimNextRun(ctx, crawlrun.ClaimNextRunInput{OrgID: org, AccountID: acct2, Now: fixedNow})
	if err != nil || !ok {
		t.Fatalf("second claim: ok=%v err=%v", ok, err)
	}
	if first.RunID == second.RunID {
		t.Fatalf("two accounts claimed the same run %d", first.RunID)
	}
}
