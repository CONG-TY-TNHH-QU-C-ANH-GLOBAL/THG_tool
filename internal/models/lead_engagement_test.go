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

	// Test helpers default to Outcome="succeeded" because the
	// autonomous-verified-execution model (project goal, May-2026)
	// makes the badge derive ONLY from verified touches. An entry
	// without an outcome is treated as not-a-touch by
	// filterVerifiedTouches. Tests that want to assert "failed
	// attempts must NOT count" build their entries inline with the
	// failing outcome explicitly.
	mkInbox := func(at time.Time) LeadEngagement {
		return LeadEngagement{Action: "inbox", PerformedAt: at, UserName: "Alice", Outcome: "succeeded"}
	}
	mkComment := func(at time.Time) LeadEngagement {
		return LeadEngagement{Action: "comment", PerformedAt: at, UserName: "Bob", Outcome: "succeeded"}
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
	entries := []LeadEngagement{{Action: "inbox", PerformedAt: now.Add(-2 * time.Minute), Outcome: "succeeded"}}
	got := DeriveBadge(entries, "", false, now, 0, 0)
	if got != LeadBadgeProtected {
		t.Errorf("zero windows must default; got %s want %s", got, LeadBadgeProtected)
	}
}

// AUTONOMOUS-VERIFIED-EXECUTION INVARIANT (project goal, May-2026):
// The "Đã chạm" / engagement badge derives ONLY from action_ledger rows
// whose outcome reached verified success. Anything else — queued
// attempts in flight, failed callbacks, context_drift aborts, blocked
// banners, rate-limit rejects — must NOT promote the lead to
// touched/protected. This was the bug behind the May-2026 screenshot
// where lead #51's `redirected_feed` outcome still rendered as
// "ĐÃ CHẠM" because the badge derivation only counted len(entries)>0.
func TestDeriveBadge_OnlyVerifiedTouchesCount(t *testing.T) {
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	fiveMinAgo := now.Add(-5 * time.Minute)

	cases := []struct {
		name    string
		entries []LeadEngagement
		want    LeadEngagementBadge
	}{
		{
			name: "queued in flight does NOT count as touched",
			entries: []LeadEngagement{{
				Action: "comment", PerformedAt: fiveMinAgo, Outcome: "queued",
			}},
			want: LeadBadgePriority,
		},
		{
			name: "failed attempt does NOT count as touched",
			entries: []LeadEngagement{{
				Action: "comment", PerformedAt: fiveMinAgo, Outcome: "failed",
			}},
			want: LeadBadgePriority,
		},
		{
			name: "skipped attempt does NOT count as touched",
			entries: []LeadEngagement{{
				Action: "comment", PerformedAt: fiveMinAgo, Outcome: "skipped",
			}},
			want: LeadBadgePriority,
		},
		{
			name: "succeeded attempt counts as touched (recent → protected)",
			entries: []LeadEngagement{{
				Action: "comment", PerformedAt: fiveMinAgo, Outcome: "succeeded",
			}},
			want: LeadBadgeProtected,
		},
		{
			name: "mix: failed (newest) + succeeded (older) — succeeded wins, picks badge from it",
			entries: []LeadEngagement{
				{Action: "comment", PerformedAt: now.Add(-1 * time.Minute), Outcome: "failed"},
				{Action: "comment", PerformedAt: fiveMinAgo, Outcome: "succeeded"},
			},
			// After filter, only the succeeded one remains. 5 min ago < protected window.
			want: LeadBadgeProtected,
		},
		{
			name: "INCIDENT scenario: lead has only a redirected_feed-mapped failed entry → not touched",
			entries: []LeadEngagement{{
				Action: "comment", PerformedAt: fiveMinAgo, Outcome: "failed",
			}},
			want: LeadBadgePriority,
		},
		{
			name: "empty outcome string defaults to NOT-touched (defensive)",
			entries: []LeadEngagement{{
				Action: "comment", PerformedAt: fiveMinAgo, Outcome: "",
			}},
			want: LeadBadgePriority,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveBadge(tc.entries, "", false, now, DefaultProtectedWindow, DefaultFollowupWindow)
			if got != tc.want {
				t.Errorf("DeriveBadge = %s, want %s", got, tc.want)
			}
		})
	}
}

// IsLedgerOutcomeVerifiedTouch is the single-source-of-truth helper.
// Pinning it via a test guards against accidental scope drift (someone
// adding "queued" to the truth set would break the screenshot fix).
func TestIsLedgerOutcomeVerifiedTouch(t *testing.T) {
	cases := map[string]bool{
		"succeeded":      true,
		"failed":         false,
		"queued":         false,
		"skipped":        false,
		"":              false,
		"context_drift": false,
		"blocked":       false,
		"rate_limited":  false,
		// Defensive: typos / future additions must default to false
		// (fail-closed: don't promote to touched unless explicitly verified).
		"SUCCEEDED": false,
		"  succeeded  ": false,
	}
	for outcome, want := range cases {
		t.Run(outcome, func(t *testing.T) {
			if got := IsLedgerOutcomeVerifiedTouch(outcome); got != want {
				t.Errorf("IsLedgerOutcomeVerifiedTouch(%q) = %v; want %v", outcome, got, want)
			}
		})
	}
}

// TerminalFromOutcome maps the rich verifier taxonomy to the
// (ExecutionState, VerificationOutcome) pair on the outbound row.
// Pin the mapping so the dashboard's per-failure-type drill-down
// stays correct as new outcomes are added.
func TestTerminalFromOutcome(t *testing.T) {
	type want struct {
		state   ExecutionState
		outcome VerificationOutcome
	}
	cases := map[ExecutionOutcome]want{
		ExecutionDOMVerified:         {ExecFinished, VerifVerifiedSuccess},
		ExecutionOptimisticSuccess:   {ExecFinished, VerifVerifiedSuccess},
		ExecutionDuplicateBlocked:    {ExecFinished, VerifVerifiedSuccess},
		ExecutionContextDrift:        {ExecFinished, VerifContextDrift},
		ExecutionRedirectedFeed:      {ExecFinished, VerifContextDrift},
		ExecutionBlocked:             {ExecFinished, VerifBlocked},
		ExecutionRateLimited:         {ExecFinished, VerifRateLimited},
		ExecutionRetryExhausted:      {ExecExpired, ""},
		ExecutionShadowRejected:      {ExecFinished, VerifShadowRejected},
		ExecutionCaptcha:             {ExecFinished, VerifCaptcha},
		ExecutionSoftFail:            {ExecFinished, VerifExecutionFailed},
		ExecutionHardFail:            {ExecFinished, VerifExecutionFailed},
		ExecutionVerificationTimeout: {ExecFinished, VerifExecutionFailed},
		ExecutionComposerFailed:      {ExecFinished, VerifExecutionFailed},
	}
	for outcome, w := range cases {
		t.Run(string(outcome), func(t *testing.T) {
			gotState, gotOutcome := TerminalFromOutcome(outcome)
			if gotState != w.state || gotOutcome != w.outcome {
				t.Errorf("TerminalFromOutcome(%s) = (%s, %s), want (%s, %s)",
					outcome, gotState, gotOutcome, w.state, w.outcome)
			}
		})
	}
}
