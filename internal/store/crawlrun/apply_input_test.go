package crawlrun

import (
	"errors"
	"testing"
	"time"
)

func validInput() ApplyRunResultInput {
	return ApplyRunResultInput{
		Fence:          Fence{OrgID: 1, RunID: 2, Attempt: 1},
		Status:         TerminalSucceeded,
		ExitReasonCode: "frontier_reached",
		Counters:       RunCounters{PostsSeen: 3, FreshLeadCount: 1},
		Leads:          []LeadCandidate{{PostDedupHash: "h1"}},
		Now:            time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
	}
}

func TestApplyInputValidate(t *testing.T) {
	if err := validInput().validate(); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}

	bad := validInput()
	bad.Fence.Attempt = 0
	if err := bad.validate(); !errors.Is(err, ErrInvalidFence) {
		t.Fatalf("want ErrInvalidFence, got %v", err)
	}

	bad = validInput()
	bad.Status = "abandoned" // reaper-owned; not worker-reportable
	if err := bad.validate(); !errors.Is(err, ErrInvalidRunResultInput) {
		t.Fatalf("want ErrInvalidRunResultInput for abandoned, got %v", err)
	}

	bad = validInput()
	bad.Status = TerminalFailed
	bad.ExitReasonCode = ""
	if err := bad.validate(); !errors.Is(err, ErrInvalidRunResultInput) {
		t.Fatalf("want ErrInvalidRunResultInput for missing reason, got %v", err)
	}

	ok := validInput()
	ok.Status = TerminalSucceeded
	ok.ExitReasonCode = "" // clean success may omit the reason
	if err := ok.validate(); err != nil {
		t.Fatalf("clean success rejected: %v", err)
	}

	bad = validInput()
	bad.Counters.DuplicateCount = -1
	if err := bad.validate(); !errors.Is(err, ErrInvalidRunResultInput) {
		t.Fatalf("want ErrInvalidRunResultInput for negative counter, got %v", err)
	}

	bad = validInput()
	bad.Now = time.Time{}
	if err := bad.validate(); !errors.Is(err, ErrInvalidRunResultInput) {
		t.Fatalf("want ErrInvalidRunResultInput for zero Now, got %v", err)
	}
}

func TestNormalizeCandidates(t *testing.T) {
	hashes, dups, err := normalizeCandidates([]LeadCandidate{
		{PostDedupHash: "a"}, {PostDedupHash: "b"}, {PostDedupHash: "a"}, {PostDedupHash: "a"},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(hashes) != 2 || hashes[0] != "a" || hashes[1] != "b" {
		t.Fatalf("want first-seen order [a b], got %v", hashes)
	}
	if dups != 2 {
		t.Fatalf("want 2 in-batch duplicates, got %d", dups)
	}

	if _, _, err := normalizeCandidates([]LeadCandidate{{PostDedupHash: ""}}); !errors.Is(err, ErrInvalidRunResultInput) {
		t.Fatalf("want ErrInvalidRunResultInput for empty hash, got %v", err)
	}
	if _, _, err := normalizeCandidates([]LeadCandidate{{PostDedupHash: " padded "}}); !errors.Is(err, ErrInvalidRunResultInput) {
		t.Fatalf("want ErrInvalidRunResultInput for padded hash, got %v", err)
	}
}

func TestClassifyApply(t *testing.T) {
	in := validInput()
	base := appliedRun{sourceID: 9, status: "running", attempt: in.Fence.Attempt,
		exitReasonCode: "", counters: RunCounters{}}

	if got := classifyApply(base, in); got != ApplyApplied {
		t.Fatalf("running row: want applied, got %s", got)
	}

	stale := base
	stale.attempt = in.Fence.Attempt + 1
	if got := classifyApply(stale, in); got != ApplyStaleRejected {
		t.Fatalf("newer attempt: want stale_rejected, got %s", got)
	}

	exact := base
	exact.status = string(in.Status)
	exact.exitReasonCode = in.ExitReasonCode
	exact.counters = in.Counters
	if got := classifyApply(exact, in); got != ApplyAlreadyApplied {
		t.Fatalf("exact terminal replay: want already_applied, got %s", got)
	}

	conflict := exact
	conflict.counters.PostsSeen++
	if got := classifyApply(conflict, in); got != ApplyConflictingReplay {
		t.Fatalf("different counters: want conflicting_replay, got %s", got)
	}
	conflict = exact
	conflict.status = string(TerminalFailed)
	if got := classifyApply(conflict, in); got != ApplyConflictingReplay {
		t.Fatalf("different terminal status: want conflicting_replay, got %s", got)
	}
	reaped := exact
	reaped.status = "abandoned"
	if got := classifyApply(reaped, in); got != ApplyConflictingReplay {
		t.Fatalf("reaper won: want conflicting_replay, got %s", got)
	}

	open := base
	open.status = "queued"
	if got := classifyApply(open, in); got != ApplyRunNotRunning {
		t.Fatalf("queued row: want run_not_running, got %s", got)
	}
}
