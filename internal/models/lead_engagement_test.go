package models

import (
	"testing"
	"time"
)

// DeriveBadge encodes the PR-4 product semantics. This is the table the
// dashboard list view depends on; keep the matrix exhaustive.
func TestDeriveBadge(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	fiveMinAgo := now.Add(-5 * time.Minute)
	twoHoursAgo := now.Add(-2 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	mkInbox := func(at time.Time) LeadEngagement {
		return LeadEngagement{Action: "inbox", PerformedAt: at, UserName: "Alice"}
	}
	mkComment := func(at time.Time) LeadEngagement {
		return LeadEngagement{Action: "comment", PerformedAt: at, UserName: "Bob"}
	}

	cases := []struct {
		name             string
		entries          []LeadEngagement
		threadStatus     string
		awaitingReply    bool
		want             LeadEngagementBadge
	}{
		{
			name:    "untouched lead is priority",
			entries: nil,
			want:    LeadBadgePriority,
		},
		{
			name:    "recent action is protected",
			entries: []LeadEngagement{mkInbox(fiveMinAgo)},
			want:    LeadBadgeProtected,
		},
		{
			name:          "inbox awaiting reply within 24h is followup_pending",
			entries:       []LeadEngagement{mkInbox(twoHoursAgo)},
			awaitingReply: true,
			want:          LeadBadgeFollowupPending,
		},
		{
			name:          "inbox awaiting reply beyond 24h falls back to visible",
			entries:       []LeadEngagement{mkInbox(twoDaysAgo)},
			awaitingReply: true,
			want:          LeadBadgeVisible,
		},
		{
			name:    "comment from 2h ago without thread is visible",
			entries: []LeadEngagement{mkComment(twoHoursAgo)},
			want:    LeadBadgeVisible,
		},
		{
			name:         "closed thread overrides every other state",
			entries:      []LeadEngagement{mkInbox(fiveMinAgo)},
			threadStatus: "closed",
			want:         LeadBadgeClosed,
		},
		{
			name:         "converted thread also resolves to closed",
			entries:      []LeadEngagement{mkInbox(twoHoursAgo)},
			threadStatus: "converted",
			want:         LeadBadgeClosed,
		},
		{
			name:         "thread initiated but no entries is still priority",
			entries:      nil,
			threadStatus: "initiated",
			want:         LeadBadgePriority,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveBadge(tc.entries, tc.threadStatus, tc.awaitingReply, now,
				DefaultProtectedWindow, DefaultFollowupWindow)
			if got != tc.want {
				t.Errorf("DeriveBadge = %s, want %s", got, tc.want)
			}
		})
	}
}

// Protected window of zero must fall back to the default so the
// dashboard never accidentally classifies every lead as not-protected.
func TestDeriveBadge_DefaultsZeroWindowsToConstants(t *testing.T) {
	now := time.Now().UTC()
	entries := []LeadEngagement{{Action: "inbox", PerformedAt: now.Add(-2 * time.Minute)}}
	got := DeriveBadge(entries, "", false, now, 0, 0)
	if got != LeadBadgeProtected {
		t.Errorf("zero windows must default; got %s want %s", got, LeadBadgeProtected)
	}
}
