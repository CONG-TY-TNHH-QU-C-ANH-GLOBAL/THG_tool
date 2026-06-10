package models

import "time"

// Auto-Archive decision (spec: specs/LEAD_LIFECYCLE_WORK_QUEUE.md, PR-3). Pure function
// over the lifecycle + coverage projections so the maintenance sweep is deterministic and
// testable. Time-based reasons use archive_after_days — distinct from stale_after_days,
// which only drives the DISPLAY state. A lead with a recent reply (live conversation) is
// never auto-archived. No hard delete: archiving only flips archived_at; the engagement
// ledger stays for dedup + multi-actor coverage history.

// ArchiveInputs are the explicit signals the sweep feeds the decision. CoverageFull and
// InvalidTarget are computed by the caller (store) from the policy + lead, keeping this
// function free of policy-shape and platform-URL knowledge.
type ArchiveInputs struct {
	Lifecycle     LeadLifecycleState
	Coverage      LeadCoverageState
	CoverageFull  bool // org reached MaxAccountsPerLead on this lead
	InvalidTarget bool // lead has no actionable target URL
}

// ArchiveSweepReport summarises one sweep over an org's leads.
type ArchiveSweepReport struct {
	Scanned  int            `json:"scanned"`
	Archived int            `json:"archived"`
	ByReason map[string]int `json:"by_reason"`
}

func archiveCutoffDays(policy LeadLifecyclePolicy) int {
	if policy.ArchiveAfterDays > 0 {
		return policy.ArchiveAfterDays
	}
	return 30
}

// EvaluateArchive decides whether a lead should be auto-archived and why. Order matters —
// first match wins; every branch is an explicit field/timestamp test.
func EvaluateArchive(in ArchiveInputs, policy LeadLifecyclePolicy, now time.Time) (bool, string) {
	if !in.Lifecycle.ArchivedAt.IsZero() {
		return false, "" // already archived
	}
	if in.InvalidTarget {
		return true, ArchiveReasonInvalidTarget
	}
	cutoff := now.Add(-time.Duration(archiveCutoffDays(policy)) * 24 * time.Hour)

	// Coverage saturated and the lead never replied, past the archive window: the org has
	// covered it as much as policy allows and it went nowhere.
	if in.CoverageFull && !in.Coverage.LeadReplied &&
		(in.Lifecycle.LastEngagedAt.IsZero() || in.Lifecycle.LastEngagedAt.Before(cutoff)) {
		return true, ArchiveReasonCoverageNoReply
	}

	// Nothing has happened for the whole archive window.
	if in.Lifecycle.LastSeenAt.Before(cutoff) {
		if in.Coverage.LeadReplied {
			return true, ArchiveReasonThreadInactive // replied once, then went quiet
		}
		return true, ArchiveReasonCold
	}
	return false, ""
}
