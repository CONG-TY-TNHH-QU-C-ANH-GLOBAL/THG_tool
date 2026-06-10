package models

import (
	"testing"
	"time"
)

func item(id int64, score LeadScore, state LeadFreshnessState, nextAt time.Time) WorkQueueItem {
	return WorkQueueItem{
		Lead:      Lead{ID: id, Score: score},
		Lifecycle: LeadLifecycleState{FreshnessState: state, NextActionAt: nextAt},
	}
}

// Ordering is score → freshness → next_action_at → id.
func TestSortWorkQueue_Ordering(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	items := []WorkQueueItem{
		item(1, LeadWarm, LeadActive, now),
		item(2, LeadHot, LeadFollowupDue, now),
		item(3, LeadHot, LeadActive, now.Add(2*time.Hour)),
		item(4, LeadHot, LeadActive, now), // earliest due among hot+active
	}
	SortWorkQueue(items)
	gotOrder := []int64{items[0].Lead.ID, items[1].Lead.ID, items[2].Lead.ID, items[3].Lead.ID}
	want := []int64{4, 3, 2, 1} // hot/active/earliest, hot/active/later, hot/followup, warm
	for i := range want {
		if gotOrder[i] != want[i] {
			t.Fatalf("order = %v, want %v", gotOrder, want)
		}
	}
}

// StateAllowed: default hides archived + stale; IncludeStale opts stale in.
func TestWorkQueueOptions_StateAllowed(t *testing.T) {
	def := WorkQueueOptions{}
	if !def.StateAllowed(LeadActive) || !def.StateAllowed(LeadFollowupDue) {
		t.Error("default should allow active + followup_due")
	}
	if def.StateAllowed(LeadArchived) {
		t.Error("archived must never be allowed")
	}
	if def.StateAllowed(LeadStale) {
		t.Error("stale hidden by default")
	}
	withStale := WorkQueueOptions{IncludeStale: true}
	if !withStale.StateAllowed(LeadStale) {
		t.Error("IncludeStale should surface stale")
	}
	if withStale.StateAllowed(LeadArchived) {
		t.Error("IncludeStale must not surface archived")
	}
	// waiting_reply is not in the default act-now set.
	if def.StateAllowed(LeadWaitingReply) {
		t.Error("waiting_reply not in default set")
	}
}
