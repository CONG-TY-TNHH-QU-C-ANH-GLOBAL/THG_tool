package leadoutreach

import (
	"fmt"
	"testing"
)

// TestOutreachMode pins the queue-result mode label extracted from the (previously
// duplicated) switch shared by queueLeadOutreach and queueFacebookPostTargets.
func TestOutreachMode(t *testing.T) {
	tests := []struct {
		name          string
		approvedCount int
		queued        int
		requestedAuto bool
		want          string
	}{
		{name: "all approved", approvedCount: 3, queued: 3, want: "approved_auto"},
		{name: "some approved is mixed", approvedCount: 1, queued: 3, want: "mixed"},
		{name: "none approved, auto requested", approvedCount: 0, queued: 3, requestedAuto: true, want: "draft_org_not_auto"},
		{name: "none approved, no auto requested", approvedCount: 0, queued: 3, want: "draft"},
		{name: "nothing queued, no auto", approvedCount: 0, queued: 0, want: "draft"},
		{name: "nothing queued, auto requested", approvedCount: 0, queued: 0, requestedAuto: true, want: "draft_org_not_auto"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mode(tt.approvedCount, tt.queued, tt.requestedAuto); got != tt.want {
				t.Errorf("Mode(%d, %d, %v) = %q, want %q", tt.approvedCount, tt.queued, tt.requestedAuto, got, tt.want)
			}
		})
	}
}

// TestOutreachErrDetails pins the " | Last Error: ..." suffix (empty when no error).
func TestOutreachErrDetails(t *testing.T) {
	if got := outreachErrDetails(nil); got != "" {
		t.Errorf("nil error = %q, want empty", got)
	}
	if got := outreachErrDetails(fmt.Errorf("boom")); got != " | Last Error: boom" {
		t.Errorf("got %q, want %q", got, " | Last Error: boom")
	}
}

// TestOutreachRiskDetails pins the risk_block suffix (empty unless a risk block was seen).
func TestOutreachRiskDetails(t *testing.T) {
	if got := outreachRiskDetails(&State{}, 42); got != "" {
		t.Errorf("no risk block = %q, want empty", got)
	}
	st := &State{riskBlockSeen: true, riskBlockRisk: 0.9, riskBlockCeiling: 0.5}
	want := " risk_block=account=42,risk_score=0.900,effective_ceiling=0.500"
	if got := outreachRiskDetails(st, 42); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
