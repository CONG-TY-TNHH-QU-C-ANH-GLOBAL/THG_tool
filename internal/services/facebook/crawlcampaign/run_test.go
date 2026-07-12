package crawlcampaign

import "testing"

func TestRunStatusClassification(t *testing.T) {
	cases := []struct {
		status   RunStatus
		open     bool
		terminal bool
	}{
		{RunQueued, true, false},
		{RunWaitingForConnectorUpgrade, true, false},
		{RunRunning, true, false},
		{RunSucceeded, false, true},
		{RunStoppedSafe, false, true},
		{RunFailed, false, true},
		{RunAbandoned, false, true},
		{RunCancelled, false, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := tc.status.IsOpen(); got != tc.open {
				t.Errorf("IsOpen() = %v, want %v", got, tc.open)
			}
			if got := tc.status.IsTerminal(); got != tc.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tc.terminal)
			}
		})
	}
}
