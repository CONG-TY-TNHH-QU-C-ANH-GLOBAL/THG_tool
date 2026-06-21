package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func parityFinalize(t *testing.T, h parityHarness) {
	const org = int64(1103)
	id := h.seedPlanned(t, org, 11, "comment", "https://fb.com/f1")
	claim, err := h.repo.ClaimPlannedOutboundForOrg(org, id, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	assertFinalizeRejectsWrongToken(t, h, org, id, claim.ExecutionID)

	before := time.Now()
	assertFinalizeSucceeds(t, h, org, id, claim.ExecutionID)
	assertSentAtStamped(t, h, org, before, time.Now())

	assertReplayIdempotent(t, h, org, id, claim.ExecutionID)
}

// assertFinalizeRejectsWrongToken finalizes with a bogus token and asserts the
// row is untouched: finalized=false, the real execID is returned, still executing.
func assertFinalizeRejectsWrongToken(t *testing.T, h parityHarness, org, id int64, realExecID string) {
	t.Helper()
	ok, _, _, curExec, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, id, "wrong-token", models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("finalize wrong-token: %v", err)
	}
	if ok {
		t.Fatalf("wrong execution_id must not finalize")
	}
	if curExec != realExecID {
		t.Fatalf("disambiguation must return current execID %q, got %q", realExecID, curExec)
	}
	if ex := readExpect(t, h, org, models.ExecExecuting, "", 10, 1); ex[0].ID != id {
		t.Fatalf("row must remain executing after wrong-token finalize")
	}
}

// assertFinalizeSucceeds finalizes with the real token and asserts the terminal
// tuple (finalized, finished, verified_success, same execID).
func assertFinalizeSucceeds(t *testing.T, h parityHarness, org, id int64, execID string) {
	t.Helper()
	ok, state, outcome, gotExec, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, id, execID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !ok || state != models.ExecFinished || outcome != models.VerifVerifiedSuccess || gotExec != execID {
		t.Fatalf("finalize wrong: ok=%v state=%q outcome=%q exec=%q", ok, state, outcome, gotExec)
	}
}

// assertSentAtStamped reads the finished row and asserts the outcome round-trips
// plus a precision-safe sent_at inside the finalize window.
func assertSentAtStamped(t *testing.T, h parityHarness, org int64, before, after time.Time) {
	t.Helper()
	fin := readExpect(t, h, org, models.ExecFinished, "", 10, 1)
	if fin[0].VerificationOutcome != models.VerifVerifiedSuccess {
		t.Fatalf("outcome must round-trip: %+v", fin[0])
	}
	if fin[0].SentAt.IsZero() {
		t.Fatalf("verified_success must stamp a non-zero sent_at")
	}
	if fin[0].SentAt.Before(before.Add(-2*time.Second)) || fin[0].SentAt.After(after.Add(2*time.Second)) {
		t.Fatalf("sent_at %v outside finalize window [%v, %v]", fin[0].SentAt, before, after)
	}
}

// assertReplayIdempotent replays the same finalize and asserts it is a no-op
// (finalized=false, state still finished).
func assertReplayIdempotent(t *testing.T, h parityHarness, org, id int64, execID string) {
	t.Helper()
	ok, st, _, _, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, id, execID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if ok || st != models.ExecFinished {
		t.Fatalf("replay must be finalized=false with current state finished, got ok=%v st=%q", ok, st)
	}
}

func parityResetStale(t *testing.T, h parityHarness) {
	const org = int64(1104)
	stale := h.seedPlanned(t, org, 11, "comment", "https://fb.com/rs1")
	fresh := h.seedPlanned(t, org, 11, "comment", "https://fb.com/rs2")
	term := h.seedPlanned(t, org, 11, "comment", "https://fb.com/rs3")

	if _, err := h.repo.ClaimPlannedOutboundForOrg(org, stale, "worker-a", time.Minute); err != nil {
		t.Fatalf("claim stale: %v", err)
	}
	freshClaim, err := h.repo.ClaimPlannedOutboundForOrg(org, fresh, "worker-b", 10*time.Minute)
	if err != nil {
		t.Fatalf("claim fresh: %v", err)
	}
	// Terminal row: claimed then finalized — must NOT be reset.
	termClaim, err := h.repo.ClaimPlannedOutboundForOrg(org, term, "worker-c", time.Minute)
	if err != nil {
		t.Fatalf("claim term: %v", err)
	}
	if ok, _, _, _, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, term, termClaim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess); err != nil || !ok {
		t.Fatalf("finalize term: ok=%v err=%v", ok, err)
	}

	// Deterministically expire only the stale row's lease (backend SQL).
	h.makeStale(t, org, stale)

	if err := h.repo.ResetStaleExecutingForOrg(org, time.Minute); err != nil {
		t.Fatalf("reset: %v", err)
	}

	planned, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecPlanned, "", 10)
	if err != nil {
		t.Fatalf("read planned: %v", err)
	}
	if len(planned) != 1 || planned[0].ID != stale {
		t.Fatalf("only the stale row must reset to planned, got %+v", planned)
	}
	if planned[0].ExecutionID != "" {
		t.Fatalf("reset must clear execution_id, got %q", planned[0].ExecutionID)
	}

	executing, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecExecuting, "", 10)
	if err != nil {
		t.Fatalf("read executing: %v", err)
	}
	if len(executing) != 1 || executing[0].ID != fresh || executing[0].ExecutionID != freshClaim.ExecutionID {
		t.Fatalf("non-stale row must stay executing, got %+v", executing)
	}

	if fin, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecFinished, "", 10); err != nil || len(fin) != 1 || fin[0].ID != term {
		t.Fatalf("terminal row must not be reset, got %+v err=%v", fin, err)
	}
}

func parityOrgIsolation(t *testing.T, h parityHarness) {
	const orgA, orgB = int64(1105), int64(1106)
	idA := h.seedPlanned(t, orgA, 11, "comment", "https://fb.com/iso")

	// orgB cannot see orgA's row.
	if rows, err := h.repo.GetOutboundByExecutionStateForOrg(orgB, models.ExecPlanned, "", 10); err != nil || len(rows) != 0 {
		t.Fatalf("orgB must see no rows, got %d err=%v", len(rows), err)
	}
	// orgB cannot claim orgA's row (cross-tenant CAS miss).
	if _, err := h.repo.ClaimPlannedOutboundForOrg(orgB, idA, "worker-b", time.Minute); err == nil {
		t.Fatalf("orgB claim of orgA row must fail")
	}

	// A reset scoped to orgB must not touch orgA's stale executing row.
	claimA, err := h.repo.ClaimPlannedOutboundForOrg(orgA, idA, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("claim orgA: %v", err)
	}
	h.makeStale(t, orgA, idA)
	if err := h.repo.ResetStaleExecutingForOrg(orgB, time.Minute); err != nil {
		t.Fatalf("reset orgB: %v", err)
	}
	ex, err := h.repo.GetOutboundByExecutionStateForOrg(orgA, models.ExecExecuting, "", 10)
	if err != nil {
		t.Fatalf("read orgA executing: %v", err)
	}
	if len(ex) != 1 || ex[0].ExecutionID != claimA.ExecutionID {
		t.Fatalf("orgA executing row must survive an orgB-scoped reset, got %+v", ex)
	}
}
