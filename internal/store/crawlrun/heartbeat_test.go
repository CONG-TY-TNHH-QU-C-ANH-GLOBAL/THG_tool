package crawlrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/crawlrun"
)

func wantOutcome(t *testing.T, got crawlrun.HeartbeatOutcome, err error, want crawlrun.HeartbeatOutcome) {
	t.Helper()
	if err != nil {
		t.Fatalf("heartbeat: unexpected error %v", err)
	}
	if got != want {
		t.Fatalf("heartbeat outcome = %q, want %q", got, want)
	}
}

func TestHeartbeat_MatchingFenceUpdated(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 43001
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	fence := crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt}
	got, err := st.Heartbeat(ctx, fence, fixedNow.Add(30*time.Second))
	wantOutcome(t, got, err, crawlrun.HeartbeatUpdated)
}

func TestHeartbeat_StaleFencesRejected(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 43002
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	cases := []struct {
		name  string
		fence crawlrun.Fence
	}{
		{"wrong org", crawlrun.Fence{OrgID: org + 1, RunID: run.RunID, Attempt: run.Attempt}},
		{"wrong run id", crawlrun.Fence{OrgID: org, RunID: run.RunID + 99999, Attempt: run.Attempt}},
		{"wrong attempt", crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt + 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := st.Heartbeat(ctx, tc.fence, fixedNow)
			wantOutcome(t, got, err, crawlrun.HeartbeatStaleRejected)
		})
	}
}

func TestHeartbeat_TerminalRunRejected(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 43003
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)
	if _, err := db.ExecContext(ctx,
		`UPDATE facebook_crawl_runs SET status='succeeded', finished_at=$3 WHERE org_id=$1 AND id=$2`,
		org, run.RunID, fixedNow); err != nil {
		t.Fatalf("mark terminal: %v", err)
	}

	fence := crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt}
	got, err := st.Heartbeat(ctx, fence, fixedNow)
	wantOutcome(t, got, err, crawlrun.HeartbeatStaleRejected)
}

func TestHeartbeat_DatabaseFailureReturnsError(t *testing.T) {
	st, db := open(t)
	const org = 43004
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel() // a cancelled context makes the exec fail at the database layer

	fence := crawlrun.Fence{OrgID: org, RunID: run.RunID, Attempt: run.Attempt}
	got, err := st.Heartbeat(cancelled, fence, fixedNow)
	if err == nil {
		t.Fatal("a database failure must surface an error")
	}
	if got != "" {
		t.Fatalf("outcome on error = %q, want the zero value", got)
	}
}
