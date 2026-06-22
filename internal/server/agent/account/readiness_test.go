package account

import (
	"testing"
)

// PR-D: a seeded account with NO online connector is not ready for any capability,
// and every capability reports the connector_offline reason.
func TestBuildAccountReadinessMatrix_NoConnector(t *testing.T) {
	found := seedUnassignedAccountReadiness(t, "readiness_matrix")
	if len(found.Capabilities) != 4 {
		t.Fatalf("want 4 capabilities, got %d", len(found.Capabilities))
	}
	for _, cap := range found.Capabilities {
		if cap.Can {
			t.Fatalf("capability %q should not be ready (no connector)", cap.Capability)
		}
		if len(cap.Reasons) == 0 || cap.Reasons[0] != "connector_offline" {
			t.Fatalf("capability %q reasons = %v, want connector_offline first", cap.Capability, cap.Reasons)
		}
	}
	if found.RequiredAction == "" {
		t.Fatalf("required_action should hint at the connector fix")
	}
}
