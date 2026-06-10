package models

import (
	"testing"
	"time"
)

// DeriveLeadLifecycle is the work-management state machine the dashboard tabs +
// work queue depend on. Keep this matrix exhaustive (one row per branch).
func TestDeriveLeadLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	policy := DefaultLeadLifecyclePolicy() // stale=14d, followup=24h
	hoursAgo := func(h int) time.Time { return now.Add(-time.Duration(h) * time.Hour) }
	daysAgo := func(d int) time.Time { return now.Add(-time.Duration(d) * 24 * time.Hour) }

	cases := []struct {
		name       string
		in         LeadLifecycleInputs
		wantState  LeadFreshnessState
		wantAction LeadNextAction
	}{
		{
			name:       "fresh untouched lead is active/comment",
			in:         LeadLifecycleInputs{LastCrawledAt: hoursAgo(1)},
			wantState:  LeadActive,
			wantAction: NextActionComment,
		},
		{
			name:       "old untouched lead is stale/archive",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(20)},
			wantState:  LeadStale,
			wantAction: NextActionArchive,
		},
		{
			name:       "touched, no reply, inside 24h window is waiting_reply/wait",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(1), LastEngagedAt: hoursAgo(2)},
			wantState:  LeadWaitingReply,
			wantAction: NextActionWait,
		},
		{
			name:       "touched, no reply, past window but not stale is followup_due/followup",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(3), LastEngagedAt: daysAgo(2)},
			wantState:  LeadFollowupDue,
			wantAction: NextActionFollowup,
		},
		{
			name:       "touched long ago, no reply, aged out is stale/archive",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(40), LastEngagedAt: daysAgo(20)},
			wantState:  LeadStale,
			wantAction: NextActionArchive,
		},
		{
			name:       "customer replied after our touch is active/reply (never stale)",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(30), LastEngagedAt: daysAgo(25), LastCustomerReplyAt: daysAgo(20)},
			wantState:  LeadActive,
			wantAction: NextActionReply,
		},
		{
			name:       "recent soft touch is waiting_verification (do not re-comment)",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(1), LastSoftTouchAt: now.Add(-10 * time.Minute)},
			wantState:  LeadWaitingVerification,
			wantAction: NextActionVerifyLater,
		},
		{
			name:       "soft touch past cooldown falls back to active (retry eligible)",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(1), LastSoftTouchAt: now.Add(-40 * time.Minute)},
			wantState:  LeadActive,
			wantAction: NextActionComment,
		},
		{
			name:       "hard verified touch newer than a recent soft touch wins",
			in:         LeadLifecycleInputs{LastCrawledAt: daysAgo(1), LastSoftTouchAt: now.Add(-10 * time.Minute), LastEngagedAt: now.Add(-5 * time.Minute)},
			wantState:  LeadWaitingReply,
			wantAction: NextActionWait,
		},
		{
			name:       "archived wins over everything",
			in:         LeadLifecycleInputs{LastCrawledAt: hoursAgo(1), ArchivedAt: hoursAgo(3), ArchiveReason: ArchiveReasonNotRelevant},
			wantState:  LeadArchived,
			wantAction: NextActionNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveLeadLifecycle(tc.in, policy, now)
			if got.FreshnessState != tc.wantState {
				t.Errorf("freshness_state = %s, want %s", got.FreshnessState, tc.wantState)
			}
			if got.NextAction != tc.wantAction {
				t.Errorf("next_action = %s, want %s", got.NextAction, tc.wantAction)
			}
		})
	}
}

// last_seen_at is the most recent of crawl / engage / reply.
func TestDeriveLeadLifecycle_LastSeenAt(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	reply := now.Add(-1 * time.Hour)
	in := LeadLifecycleInputs{
		LastCrawledAt:       now.Add(-72 * time.Hour),
		LastEngagedAt:       now.Add(-10 * time.Hour),
		LastCustomerReplyAt: reply, // most recent
	}
	got := DeriveLeadLifecycle(in, DefaultLeadLifecyclePolicy(), now)
	if !got.LastSeenAt.Equal(reply) {
		t.Errorf("last_seen_at = %v, want %v (latest signal)", got.LastSeenAt, reply)
	}
}

// waiting_reply's next_action_at is the moment the followup becomes due.
func TestDeriveLeadLifecycle_WaitDueAt(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	engaged := now.Add(-2 * time.Hour)
	in := LeadLifecycleInputs{LastCrawledAt: now.Add(-24 * time.Hour), LastEngagedAt: engaged}
	got := DeriveLeadLifecycle(in, DefaultLeadLifecyclePolicy(), now)
	wantDue := engaged.Add(DefaultFollowupWindow)
	if !got.NextActionAt.Equal(wantDue) {
		t.Errorf("next_action_at = %v, want %v", got.NextActionAt, wantDue)
	}
}
