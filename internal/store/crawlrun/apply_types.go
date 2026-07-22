package crawlrun

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidRunResultInput signals a structurally invalid ApplyRunResultInput —
// a caller/adapter bug, never a durable-state outcome. Wrapped messages carry
// the offending field; match with errors.Is.
var ErrInvalidRunResultInput = errors.New("crawlrun: invalid run result input")

// TerminalStatus is a worker-reportable terminal run status. The reaper owns
// 'abandoned' and operators own 'cancelled'; neither may arrive through
// ApplyRunResult, so they are deliberately not representable here.
type TerminalStatus string

const (
	TerminalSucceeded   TerminalStatus = "succeeded"
	TerminalStoppedSafe TerminalStatus = "stopped_safe"
	TerminalFailed      TerminalStatus = "failed"
)

func (s TerminalStatus) valid() bool {
	switch s {
	case TerminalSucceeded, TerminalStoppedSafe, TerminalFailed:
		return true
	default:
		return false
	}
}

// RunCounters are the per-run tally columns on facebook_crawl_runs. All values
// are non-negative counts observed by the worker for this attempt.
type RunCounters struct {
	PostsSeen      int
	FreshLeadCount int
	StaleSkipped   int
	DuplicateCount int
	UnparsedCount  int
}

func (c RunCounters) valid() bool {
	return c.PostsSeen >= 0 && c.FreshLeadCount >= 0 && c.StaleSkipped >= 0 &&
		c.DuplicateCount >= 0 && c.UnparsedCount >= 0
}

// LeadCandidate is one accepted fresh-lead identity to reserve in
// facebook_crawl_lead_index. Identity is (org, platform='facebook',
// PostDedupHash); the org comes from the fence, never from the candidate.
type LeadCandidate struct {
	PostDedupHash string
}

// ApplyRunResultInput carries one completed attempt's terminal result.
// NewestPostAt advances the source frontier cursor monotonically when non-zero;
// a zero value means the run observed no parseable post timestamp.
type ApplyRunResultInput struct {
	Fence          Fence
	Status         TerminalStatus
	ExitReasonCode string
	Counters       RunCounters
	Leads          []LeadCandidate
	NewestPostAt   time.Time
	Now            time.Time
}

// validate rejects structurally invalid input before any SQL runs. A non-clean
// terminal status must carry a typed exit reason (technical.md §exit codes).
func (in ApplyRunResultInput) validate() error {
	if !in.Fence.valid() {
		return ErrInvalidFence
	}
	if !in.Status.valid() {
		return fmt.Errorf("%w: terminal status %q", ErrInvalidRunResultInput, in.Status)
	}
	if in.Status != TerminalSucceeded && in.ExitReasonCode == "" {
		return fmt.Errorf("%w: %s requires an exit reason code", ErrInvalidRunResultInput, in.Status)
	}
	if !in.Counters.valid() {
		return fmt.Errorf("%w: negative counter", ErrInvalidRunResultInput)
	}
	if in.Now.IsZero() {
		return fmt.Errorf("%w: zero Now", ErrInvalidRunResultInput)
	}
	return nil
}

// normalizeCandidates validates and de-duplicates the batch, preserving first-
// seen order. Duplicate identities inside one batch are a normal crawl artifact
// (same post surfaced twice), counted rather than rejected; an empty hash is a
// caller bug.
func normalizeCandidates(leads []LeadCandidate) (hashes []string, duplicates int, err error) {
	seen := make(map[string]struct{}, len(leads))
	for _, lead := range leads {
		hash := lead.PostDedupHash
		if hash == "" || strings.TrimSpace(hash) != hash {
			return nil, 0, fmt.Errorf("%w: malformed post dedup hash %q",
				ErrInvalidRunResultInput, hash)
		}
		if _, dup := seen[hash]; dup {
			duplicates++
			continue
		}
		seen[hash] = struct{}{}
		hashes = append(hashes, hash)
	}
	return hashes, duplicates, nil
}

// ApplyResult classifies one ApplyRunResult call. Wrong org, unknown run, and a
// superseded attempt all collapse into ApplyStaleRejected so a caller can never
// probe another tenant's runs or a newer attempt (same posture as Heartbeat and
// RecoverDispatchFailure).
type ApplyResult string

const (
	// ApplyApplied: this call committed the terminal state and lead reservations.
	ApplyApplied ApplyResult = "applied"
	// ApplyAlreadyApplied: exact replay — the run is already terminal with the
	// same attempt, status, exit reason, and counters. Deterministic no-op.
	ApplyAlreadyApplied ApplyResult = "already_applied"
	// ApplyConflictingReplay: the run is already terminal under this attempt but
	// with a different status, exit reason, or counters. Nothing is mutated and
	// the caller must not treat this as success.
	ApplyConflictingReplay ApplyResult = "conflicting_replay"
	// ApplyRunNotRunning: the run exists under this fence but never reached
	// 'running' (queued / waiting_for_connector_upgrade); results cannot apply.
	ApplyRunNotRunning ApplyResult = "run_not_running"
	// ApplyStaleRejected: no run matches this org/run/attempt.
	ApplyStaleRejected ApplyResult = "stale_rejected"
)

// ApplyRunResultOutcome reports the classification plus the authorized lead
// counts. On ApplyAlreadyApplied the counts are recomputed from the committed
// state so exact replays are deterministic.
type ApplyRunResultOutcome struct {
	Result             ApplyResult
	CandidatesReceived int
	DuplicatesInBatch  int
	LeadsIndexed       int
	LeadsAlreadyKnown  int
}
