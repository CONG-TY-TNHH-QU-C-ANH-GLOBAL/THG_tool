package models

import (
	"slices"
	"sort"
)

// Work Queue (spec: specs/domains/facebook-sales-intelligence/features/lead-lifecycle/technical.md, PR-2). The dashboard- and
// planner-facing read model: the subset of leads worth acting on now, lifecycle-filtered
// and ordered. It composes the three existing projections — lifecycle (work axis),
// coverage (multi-actor axis), and per-actor eligibility — so the copilot/comment planner
// selects from THIS, not the raw lead list. Pure ordering/filtering lives here; the store
// gathers the inputs (work_queue.go in the leads subpackage).

// WorkQueueItem is one actionable lead with its projected state attached.
type WorkQueueItem struct {
	Lead          Lead               `json:"lead"`
	Lifecycle     LeadLifecycleState `json:"lifecycle"`
	Coverage      LeadCoverageState  `json:"coverage"`
	ActorEligible bool               `json:"actor_eligible"` // valid only when ActorAccountID was set
	ActorReason   string             `json:"actor_reason"`   // coverage reason code ("" when no actor context)
}

// WorkQueueOptions parameterises a work-queue read.
type WorkQueueOptions struct {
	Score           string               // hot|warm|cold|"" (all)
	Limit           int                  // max items returned (0 = no cap)
	States          []LeadFreshnessState // allowed freshness states; nil → DefaultWorkQueueStates()
	IncludeStale    bool                 // surface stale leads too (default hides them)
	IncludeArchived bool                 // explicit override: surface archived leads too (default hides them)
	ComputeCoverage bool                 // attach multi-actor coverage state (dashboard); planner path skips it
	ActorAccountID  int64                // when >0 (implies ComputeCoverage), compute per-actor eligibility
	Website         string               // org grounded website, for coverage projection
	Policy          LeadLifecyclePolicy
	CoveragePolicy  CoveragePolicy
}

// DefaultWorkQueueStates is the default visible set: act-now leads only. Archived is
// excluded upstream (GetLeadsFiltered) and stale is opt-in via IncludeStale.
func DefaultWorkQueueStates() []LeadFreshnessState {
	return []LeadFreshnessState{LeadActive, LeadFollowupDue}
}

// StateAllowed reports whether a freshness state belongs in the queue under opts.
func (o WorkQueueOptions) StateAllowed(state LeadFreshnessState) bool {
	if state == LeadArchived {
		return o.IncludeArchived // explicit override only; default hides archived
	}
	if state == LeadStale {
		return o.IncludeStale
	}
	states := o.States
	if states == nil {
		states = DefaultWorkQueueStates()
	}
	return slices.Contains(states, state)
}

// scoreRank maps the lead score to a sortable weight (hot first).
func scoreRank(score LeadScore) int {
	switch score {
	case LeadHot:
		return 3
	case LeadWarm:
		return 2
	case LeadCold:
		return 1
	default:
		return 0
	}
}

// freshnessRank orders states by product priority: the dashboard surfaces "Cần xử lý"
// (active) before "Đến hạn follow-up" (followup_due); waiting/stale rank below.
func freshnessRank(state LeadFreshnessState) int {
	switch state {
	case LeadActive:
		return 4
	case LeadFollowupDue:
		return 3
	case LeadWaitingReply:
		return 2
	case LeadWaitingVerification:
		return 2
	case LeadStale:
		return 1
	default:
		return 0
	}
}

// LessWorkQueueItem is the deterministic ordering: score → freshness → next_action_at
// (earliest due first) → lead id (stable tiebreak). Pure, so it is unit-testable.
func LessWorkQueueItem(a, b WorkQueueItem) bool {
	if ra, rb := scoreRank(a.Lead.Score), scoreRank(b.Lead.Score); ra != rb {
		return ra > rb
	}
	if ra, rb := freshnessRank(a.Lifecycle.FreshnessState), freshnessRank(b.Lifecycle.FreshnessState); ra != rb {
		return ra > rb
	}
	if !a.Lifecycle.NextActionAt.Equal(b.Lifecycle.NextActionAt) {
		return a.Lifecycle.NextActionAt.Before(b.Lifecycle.NextActionAt)
	}
	return a.Lead.ID < b.Lead.ID
}

// SortWorkQueue orders items in place by LessWorkQueueItem.
func SortWorkQueue(items []WorkQueueItem) {
	sort.SliceStable(items, func(i, j int) bool { return LessWorkQueueItem(items[i], items[j]) })
}
