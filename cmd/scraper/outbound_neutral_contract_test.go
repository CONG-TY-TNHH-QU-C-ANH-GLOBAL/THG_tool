package main

import (
	"context"
	"testing"
)

// Characterization tests for the vertical-neutral outbound contracts that the Phase C
// cut keeps in the `outbound` core (REFACTOR_ROADMAP Phase C; MODULE_BOUNDARIES
// outbound). These pin CURRENT behavior so the neutral-vs-FB split can be proven
// behavior-preserving. They assert what the code does today, NOT what it "should" do.

// queueLeadOutreach rejects a missing/invalid org_id BEFORE any account resolution or
// queueing — the neutral tenant precondition. Pinned verbatim (error, not a message).
func TestQueueLeadOutreach_RequiresOrgID(t *testing.T) {
	ctx := context.Background()
	for _, args := range []map[string]any{
		{},                    // no org_id
		{"org_id": int64(0)},  // zero
		{"org_id": int64(-3)}, // negative
	} {
		_, _, err := queueLeadOutreach(ctx, nil, nil, "comment", args, nil)
		if err == nil || err.Error() != "org_id is required for outbound automation" {
			t.Errorf("args %v: got err=%v, want \"org_id is required for outbound automation\"", args, err)
		}
	}
}
