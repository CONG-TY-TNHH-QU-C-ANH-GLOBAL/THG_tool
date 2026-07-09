package main

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/session/accountsafety"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/crawl"
	"github.com/thg/scraper/internal/store/sessions"
)

// PR-C4 scheduler ↔ Account Safety Coordinator contract over a real store:
// budget=1 admits one crawl per tick, result feedback (Finish) — not the stale
// timeout — frees the slot for the next tick, parked accounts are skipped, and
// a failed submit never leaks the machine budget.

func seedDueCrawlIntent(t *testing.T, db *store.Store, orgID, accountID int64, sourceURL string, due time.Time) *crawl.Intent {
	t.Helper()
	in, err := db.Crawl().UpsertIntent(context.Background(), crawl.Intent{
		OrgID: orgID, AccountID: accountID, Name: "safety", Prompt: "find leads",
		Intent: "facebook_crawl", SourceType: "facebook_group", SourceURL: sourceURL,
		Keywords: []string{"lead"}, IntervalMinutes: 30, MaxItems: 10, NextRunAt: due,
	})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func newSafetyCoordinator() *accountsafety.Coordinator {
	return accountsafety.NewCoordinator(accountsafety.DefaultConfig(), 15*time.Minute)
}

// seedRoutableSession records a live CDP browser session for the account so
// submitOpenCrawl falls through to the jobStore path instead of requiring an
// online Chrome-extension connector.
func seedRoutableSession(t *testing.T, db *store.Store, orgID, accountID int64) {
	t.Helper()
	now := time.Now().UTC()
	err := db.Sessions().UpsertSession(context.Background(), sessions.BrowserSession{
		AccountID: accountID, OrgID: orgID, Status: "idle", CDPPort: 9222,
		StartedAt: now, LastActiveAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Two due intents, budget=1: the first tick submits exactly one; a tick with the
// slot still held submits nothing; Finish(completed) frees the slot so the NEXT
// tick submits the waiting intent — with no 15-minute stale wait anywhere.
func TestSchedulerBudgetOneAcrossTicks(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	coord := newSafetyCoordinator()
	now := time.Now().UTC()
	seedRoutableSession(t, db, 1, 101)
	seedRoutableSession(t, db, 1, 102)
	seedDueCrawlIntent(t, db, 1, 101, "https://facebook.com/groups/one", now.Add(-2*time.Minute))
	seedDueCrawlIntent(t, db, 1, 102, "https://facebook.com/groups/two", now.Add(-time.Minute))

	if err := scheduleDueCrawlIntents(ctx, db, js, coord); err != nil {
		t.Fatal(err)
	}
	if got := countImportJobs(t, js); got != 1 {
		t.Fatalf("first tick with budget=1 submitted %d jobs, want exactly 1", got)
	}
	if got := coord.Snapshot(now).Active; got != 1 {
		t.Fatalf("active crawls = %d, want 1", got)
	}

	if err := scheduleDueCrawlIntents(ctx, db, js, coord); err != nil {
		t.Fatal(err)
	}
	if got := countImportJobs(t, js); got != 1 {
		t.Fatalf("tick with a full budget submitted %d total jobs, want still 1 (extra intent stays due)", got)
	}

	coord.Finish(101, "completed", time.Now().UTC())
	if err := scheduleDueCrawlIntents(ctx, db, js, coord); err != nil {
		t.Fatal(err)
	}
	if got := countImportJobs(t, js); got != 2 {
		t.Fatalf("after Finish(completed) the next tick submitted %d total jobs, want 2", got)
	}
}

// A checkpoint-parked account is claimed but never submitted; the mission is NOT
// killed (stays active, re-fires next interval) and runs again after Resolve.
func TestSchedulerSkipsParkedAccountUntilResolve(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	coord := newSafetyCoordinator()
	now := time.Now().UTC()
	coord.MarkRunning(201, now)
	coord.Finish(201, accountsafety.ReasonCheckpointSuspected, now)

	seedRoutableSession(t, db, 1, 201)
	in := seedDueCrawlIntent(t, db, 1, 201, "https://facebook.com/groups/parked", now.Add(-time.Minute))
	if err := scheduleDueCrawlIntents(ctx, db, js, coord); err != nil {
		t.Fatal(err)
	}
	if got := countImportJobs(t, js); got != 0 {
		t.Fatalf("parked account submitted %d jobs, want 0 (checkpoint needs operator resolution)", got)
	}
	after, err := db.Crawl().GetIntentByHash(ctx, 1, in.DedupHash)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != crawl.IntentStatusActive {
		t.Errorf("parked skip must not kill the mission: status = %s, want active", after.Status)
	}
	if !after.NextRunAt.After(now) {
		t.Error("claim must advance next_run_at so the parked mission re-fires next interval")
	}

	coord.Resolve(201)
	// Re-upsert (same dedup hash) to make the mission due again right now.
	seedDueCrawlIntent(t, db, 1, 201, "https://facebook.com/groups/parked", now.Add(-time.Minute))
	if err := scheduleDueCrawlIntents(ctx, db, js, coord); err != nil {
		t.Fatal(err)
	}
	if got := countImportJobs(t, js); got != 1 {
		t.Errorf("after operator Resolve the mission must run: %d jobs, want 1", got)
	}
}

// A failed submit must not consume the machine budget: MarkRunning only fires
// after a successful submit.
func TestSchedulerSubmitFailureDoesNotHoldSlot(t *testing.T) {
	ctx := context.Background()
	db, js := newIntakeEnv(t)
	seedDueCrawlIntent(t, db, 1, 301, "https://facebook.com/groups/fail", time.Now().UTC().Add(-time.Minute))
	if err := js.Close(); err != nil {
		t.Fatal(err)
	}
	coord := newSafetyCoordinator()
	if err := scheduleDueCrawlIntents(ctx, db, js, coord); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if got := coord.Snapshot(now).Active; got != 0 {
		t.Errorf("failed submit left %d accounts running, want 0", got)
	}
	if got := coord.FreeSlots(now); got != 1 {
		t.Errorf("failed submit left %d free slots, want the full budget of 1", got)
	}
}
