package crawlcampaign

import "testing"

func TestRunStatusClassification(t *testing.T) {
	cases := []struct {
		status   RunStatus
		valid    bool
		open     bool
		terminal bool
	}{
		{RunQueued, true, true, false},
		{RunWaitingForConnectorUpgrade, true, true, false},
		{RunRunning, true, true, false},
		{RunSucceeded, true, false, true},
		{RunStoppedSafe, true, false, true},
		{RunFailed, true, false, true},
		{RunAbandoned, true, false, true},
		{RunCancelled, true, false, true},
		{RunStatus("bogus"), false, false, false},
		{RunStatus(""), false, false, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.Valid(); got != tc.valid {
				t.Errorf("Valid() = %v, want %v", got, tc.valid)
			}
			if got := tc.status.IsOpen(); got != tc.open {
				t.Errorf("IsOpen() = %v, want %v", got, tc.open)
			}
			if got := tc.status.IsTerminal(); got != tc.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tc.terminal)
			}
		})
	}
}

func TestRunFenceValid(t *testing.T) {
	cases := []struct {
		name  string
		fence RunFence
		want  bool
	}{
		{"all positive", RunFence{OrgID: 1, RunID: 2, Attempt: 1}, true},
		{"zero org", RunFence{OrgID: 0, RunID: 2, Attempt: 1}, false},
		{"zero run", RunFence{OrgID: 1, RunID: 0, Attempt: 1}, false},
		{"zero attempt", RunFence{OrgID: 1, RunID: 2, Attempt: 0}, false},
		{"negative org", RunFence{OrgID: -1, RunID: 2, Attempt: 1}, false},
		{"negative run", RunFence{OrgID: 1, RunID: -1, Attempt: 1}, false},
		{"negative attempt", RunFence{OrgID: 1, RunID: 2, Attempt: -1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.fence.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunCountersValid(t *testing.T) {
	cases := []struct {
		name     string
		counters RunCounters
		want     bool
	}{
		{"zero", RunCounters{}, true},
		{"positive", RunCounters{PostsSeen: 40, FreshLeadCount: 3, StaleSkipped: 20, DuplicateCount: 5, UnparsedCount: 2}, true},
		{"negative posts_seen", RunCounters{PostsSeen: -1}, false},
		{"negative fresh_lead_count", RunCounters{FreshLeadCount: -1}, false},
		{"negative stale_skipped", RunCounters{StaleSkipped: -1}, false},
		{"negative duplicate_count", RunCounters{DuplicateCount: -1}, false},
		{"negative unparsed_count", RunCounters{UnparsedCount: -1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.counters.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}
