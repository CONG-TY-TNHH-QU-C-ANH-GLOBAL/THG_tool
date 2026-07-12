package crawlrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/crawlrun"
)

func TestHeartbeat_MatchingAttemptSucceeds(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 43001
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	fence := crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt}
	ok, err := st.Heartbeat(ctx, fence, fixedNow.Add(30*time.Second))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !ok {
		t.Fatal("heartbeat on the live attempt must match")
	}
}

func TestHeartbeat_StaleAttemptRejected(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 43002
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	stale := crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt + 1}
	ok, err := st.Heartbeat(ctx, stale, fixedNow)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if ok {
		t.Fatal("heartbeat from a stale attempt must not match")
	}
}

func TestHeartbeat_NonRunningRejected(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 43003
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "src-a", nil)
	out, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	queued := crawlrun.Fence{OrgID: org, RunID: out.CreatedRunIDs[0], Attempt: 1}
	ok, err := st.Heartbeat(ctx, queued, fixedNow)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if ok {
		t.Fatal("heartbeat on a queued (not running) run must not match")
	}
}
