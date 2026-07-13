package crawlrun_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store/crawlrun"
)

var fixedNow = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

func TestEnqueueDueRuns_CreatesOnceThenIdempotent(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 41001
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "src-a", nil)

	first, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow})
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if len(first.CreatedRunIDs) != 1 {
		t.Fatalf("want 1 created run, got %d (reused %d)", len(first.CreatedRunIDs), len(first.ReusedRunIDs))
	}
	if status, _ := runStatus(t, db, org, first.CreatedRunIDs[0]); status != "queued" {
		t.Fatalf("new run status = %q, want queued", status)
	}

	second, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow})
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if len(second.CreatedRunIDs) != 0 {
		t.Fatalf("re-enqueue must create nothing for an already-open source, got %d", len(second.CreatedRunIDs))
	}
}

func TestEnqueueDueRuns_SkipsSourceWithinCadence(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 41002
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	recent := fixedNow.Add(-10 * time.Minute)
	seedSource(t, db, s, "src-recent", &recent)

	out, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(out.CreatedRunIDs) != 0 {
		t.Fatalf("source last run 10m ago (cadence 240m) is not due, got %d created", len(out.CreatedRunIDs))
	}
}

func TestEnqueueDueRuns_MultipleDueSources(t *testing.T) {
	st, db := open(t)
	ctx := context.Background()
	const org = 41003
	cleanupOrg(t, db, org)

	s := seedCampaign(t, db, org, 240, 1440)
	stale := fixedNow.Add(-5 * time.Hour)
	seedSource(t, db, s, "src-1", nil)
	seedSource(t, db, s, "src-2", &stale)

	out, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(out.CreatedRunIDs) != 2 {
		t.Fatalf("want 2 due sources enqueued, got %d", len(out.CreatedRunIDs))
	}
}
