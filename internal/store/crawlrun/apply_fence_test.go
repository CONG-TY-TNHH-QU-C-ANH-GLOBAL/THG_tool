package crawlrun_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/store/crawlrun"
)

func TestApplyRunResult_StaleAttemptRejected(t *testing.T) {
	st, db := open(t)
	const org = 45011
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	stale := resultInput(run, org, "h-a")
	stale.Fence.Attempt = run.Attempt + 1 // an attempt this run never reached
	mustApply(t, st, stale, crawlrun.ApplyStaleRejected)

	if r := readRun(t, db, org, run.RunID); r.status != "running" {
		t.Fatalf("stale apply mutated run to %q", r.status)
	}
	if len(indexRows(t, db, org)) != 0 {
		t.Fatalf("stale apply created index rows")
	}
}

func TestApplyRunResult_WrongOrgRejected(t *testing.T) {
	st, db := open(t)
	const org = 45012
	const otherOrg = 45013
	cleanupOrg(t, db, org)
	cleanupOrg(t, db, otherOrg)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)

	cross := resultInput(run, otherOrg, "h-a") // real run id, foreign org fence
	mustApply(t, st, cross, crawlrun.ApplyStaleRejected)

	if r := readRun(t, db, org, run.RunID); r.status != "running" {
		t.Fatalf("cross-org apply mutated run to %q", r.status)
	}
	if len(indexRows(t, db, otherOrg))+len(indexRows(t, db, org)) != 0 {
		t.Fatalf("cross-org apply created index rows")
	}
}

func TestApplyRunResult_NonexistentRunRejected(t *testing.T) {
	st, db := open(t)
	const org = 45014
	cleanupOrg(t, db, org)

	in := crawlrun.ApplyRunResultInput{
		Fence:  crawlrun.Fence{OrgID: org, RunID: 999999999, Attempt: 1},
		Status: crawlrun.TerminalSucceeded,
		Now:    fixedNow,
	}
	mustApply(t, st, in, crawlrun.ApplyStaleRejected)
}

func TestApplyRunResult_QueuedRunNotApplicable(t *testing.T) {
	st, db := open(t)
	const org = 45015
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	seedSource(t, db, s, "queued-src", nil)
	ctx := context.Background()
	out, err := st.EnqueueDueRuns(ctx, crawlrun.EnqueueDueRunsInput{OrgID: org, Now: fixedNow})
	if err != nil || len(out.CreatedRunIDs) != 1 {
		t.Fatalf("enqueue: out=%+v err=%v", out, err)
	}
	runID := out.CreatedRunIDs[0]

	in := crawlrun.ApplyRunResultInput{
		Fence:  crawlrun.Fence{OrgID: org, RunID: runID, Attempt: 1},
		Status: crawlrun.TerminalSucceeded,
		Now:    fixedNow,
	}
	mustApply(t, st, in, crawlrun.ApplyRunNotRunning)

	if r := readRun(t, db, org, runID); r.status != "queued" {
		t.Fatalf("queued run mutated to %q", r.status)
	}
}

func TestApplyRunResult_TerminalCannotRegress(t *testing.T) {
	st, db := open(t)
	const org = 45016
	cleanupOrg(t, db, org)
	s := seedCampaign(t, db, org, 240, 1440)
	run := runningRun(t, st, db, s, fixedNow)
	mustApply(t, st, resultInput(run, org, "h-a"), crawlrun.ApplyApplied)

	// Any second write under the same fence is replay classification, never a
	// state change: the exact input is a no-op, a different one is a conflict.
	mustApply(t, st, resultInput(run, org, "h-a"), crawlrun.ApplyAlreadyApplied)
	different := resultInput(run, org, "h-a")
	different.Counters.PostsSeen++
	mustApply(t, st, different, crawlrun.ApplyConflictingReplay)

	if r := readRun(t, db, org, run.RunID); r.status != "succeeded" || r.postsSeen != 5 {
		t.Fatalf("terminal row changed: %+v", r)
	}
}

func TestApplyRunResult_SameIdentityIsolatedAcrossOrgs(t *testing.T) {
	st, db := open(t)
	const orgA = 45017
	const orgB = 45018
	cleanupOrg(t, db, orgA)
	cleanupOrg(t, db, orgB)

	runA := runningRun(t, st, db, seedCampaign(t, db, orgA, 240, 1440), fixedNow)
	runB := runningRun(t, st, db, seedCampaign(t, db, orgB, 240, 1440), fixedNow)

	outA := mustApply(t, st, resultInput(runA, orgA, "h-shared"), crawlrun.ApplyApplied)
	outB := mustApply(t, st, resultInput(runB, orgB, "h-shared"), crawlrun.ApplyApplied)
	if outA.LeadsIndexed != 1 || outB.LeadsIndexed != 1 {
		t.Fatalf("org isolation collapsed dedup: A=%+v B=%+v", outA, outB)
	}
	if indexRows(t, db, orgA)["h-shared"] != runA.RunID ||
		indexRows(t, db, orgB)["h-shared"] != runB.RunID {
		t.Fatalf("index ownership crossed orgs")
	}
}
