package models

import (
	"testing"
	"time"
)

// EvaluateArchive matrix — the auto-archive sweep depends on this. Keep it exhaustive.
func TestEvaluateArchive(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	policy := DefaultLeadLifecyclePolicy() // archive_after_days = 30
	daysAgo := func(d int) time.Time { return now.Add(-time.Duration(d) * 24 * time.Hour) }

	lc := func(lastSeen, lastEngaged time.Time) LeadLifecycleState {
		return LeadLifecycleState{LastSeenAt: lastSeen, LastEngagedAt: lastEngaged}
	}

	cases := []struct {
		name       string
		in         ArchiveInputs
		wantArch   bool
		wantReason string
	}{
		{
			name:     "fresh lead is kept",
			in:       ArchiveInputs{Lifecycle: lc(daysAgo(2), time.Time{})},
			wantArch: false,
		},
		{
			name:       "cold lead past archive window",
			in:         ArchiveInputs{Lifecycle: lc(daysAgo(40), time.Time{})},
			wantArch:   true,
			wantReason: ArchiveReasonCold,
		},
		{
			name:       "no target url is archived immediately",
			in:         ArchiveInputs{Lifecycle: lc(daysAgo(1), time.Time{}), InvalidTarget: true},
			wantArch:   true,
			wantReason: ArchiveReasonInvalidTarget,
		},
		{
			name: "coverage full, no reply, past window",
			in: ArchiveInputs{
				Lifecycle:    lc(daysAgo(35), daysAgo(35)),
				CoverageFull: true,
			},
			wantArch:   true,
			wantReason: ArchiveReasonCoverageNoReply,
		},
		{
			name: "replied then went quiet past window is thread_inactive",
			in: ArchiveInputs{
				Lifecycle: lc(daysAgo(40), daysAgo(45)),
				Coverage:  LeadCoverageState{LeadReplied: true},
			},
			wantArch:   true,
			wantReason: ArchiveReasonThreadInactive,
		},
		{
			name: "recent reply keeps a live conversation",
			in: ArchiveInputs{
				Lifecycle: lc(daysAgo(1), daysAgo(40)),
				Coverage:  LeadCoverageState{LeadReplied: true},
			},
			wantArch: false,
		},
		{
			name:     "already archived is never re-archived",
			in:       ArchiveInputs{Lifecycle: LeadLifecycleState{ArchivedAt: daysAgo(5), LastSeenAt: daysAgo(40)}},
			wantArch: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotArch, gotReason := EvaluateArchive(tc.in, policy, now)
			if gotArch != tc.wantArch {
				t.Fatalf("archived = %v, want %v", gotArch, tc.wantArch)
			}
			if tc.wantArch && gotReason != tc.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tc.wantReason)
			}
		})
	}
}
