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

	// Wrong execution_id must NOT mutate the row.
	ok, _, _, curExec, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, id, "wrong-token", models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("finalize wrong-token: %v", err)
	}
	if ok {
		t.Fatalf("wrong execution_id must not finalize")
	}
	if curExec != claim.ExecutionID {
		t.Fatalf("disambiguation must return current execID %q, got %q", claim.ExecutionID, curExec)
	}
	if ex, _ := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecExecuting, "", 10); len(ex) != 1 {
		t.Fatalf("row must remain executing after wrong-token finalize")
	}

	// Correct finalize → terminal state + outcome + sent_at stamp.
	before := time.Now()
	ok, state, outcome, execID, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	after := time.Now()
	if !ok || state != models.ExecFinished || outcome != models.VerifVerifiedSuccess || execID != claim.ExecutionID {
		t.Fatalf("finalize wrong: ok=%v state=%q outcome=%q exec=%q", ok, state, outcome, execID)
	}

	fin, err := h.repo.GetOutboundByExecutionStateForOrg(org, models.ExecFinished, "", 10)
	if err != nil {
		t.Fatalf("read finished: %v", err)
	}
	if len(fin) != 1 || fin[0].VerificationOutcome != models.VerifVerifiedSuccess {
		t.Fatalf("outcome must round-trip: %+v", fin)
	}
	// Precision-safe: sent_at is non-zero and inside the finalize window.
	if fin[0].SentAt.IsZero() {
		t.Fatalf("verified_success must stamp a non-zero sent_at")
	}
	if fin[0].SentAt.Before(before.Add(-2*time.Second)) || fin[0].SentAt.After(after.Add(2*time.Second)) {
		t.Fatalf("sent_at %v outside finalize window [%v, %v]", fin[0].SentAt, before, after)
	}

	// Replay with the same token is idempotent: finalized=false, current state.
	ok, st, _, _, err := h.repo.FinalizeOutboundAttempt(
		context.Background(), org, id, claim.ExecutionID, models.ExecFinished, models.VerifVerifiedSuccess)
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
