package connectors

import "testing"

// The four-state version classifier (PR-4) — table-driven over the
// policy floors. latest/update_available allow automation;
// update_required/unsupported block it; unknown builds are unsupported.
func TestEvaluateVersionState(t *testing.T) {
	p := VersionPolicy{
		LatestVersion:       "0.6.0",
		MinSupportedVersion: "0.5.20",
		MinRequiredVersion:  "0.5.26",
	}
	cases := []struct {
		version   string
		wantState string
		wantAllow bool
	}{
		{"0.6.0", VersionStateLatest, true},
		{"0.6.2", VersionStateLatest, true},
		{"0.5.30", VersionStateUpdateAvailable, true}, // old but supported → soft warning
		{"0.5.26", VersionStateUpdateAvailable, true},
		{"0.5.25", VersionStateUpdateRequired, false}, // below required floor → paused
		{"0.5.20", VersionStateUpdateRequired, false},
		{"0.5.19", VersionStateUnsupported, false}, // below supported floor → blocked
		{"", VersionStateUnsupported, false},       // unknown build → blocked
	}
	for _, c := range cases {
		state := EvaluateVersionState(c.version, p)
		if state != c.wantState {
			t.Errorf("EvaluateVersionState(%q) = %q, want %q", c.version, state, c.wantState)
		}
		if VersionStateAllowsAutomation(state) != c.wantAllow {
			t.Errorf("VersionStateAllowsAutomation(%q) = %v, want %v", state, !c.wantAllow, c.wantAllow)
		}
	}
}

// No latest_version configured → anything >= required floor reads as
// latest (no nag); the default policy must behave like the pre-PR-4
// single MinExtensionVersion floor.
func TestEvaluateVersionState_DefaultPolicy(t *testing.T) {
	p := DefaultVersionPolicy()
	if s := EvaluateVersionState("0.5.54", p); s != VersionStateLatest {
		t.Errorf("current build under default policy = %q, want latest", s)
	}
	if s := EvaluateVersionState("0.5.10", p); s != VersionStateUpdateRequired && s != VersionStateUnsupported {
		t.Errorf("ancient build under default policy = %q, want a blocking state", s)
	}
	if VersionStateReason(VersionStateUpdateRequired) != ConnExtensionUpdateRequired ||
		VersionStateReason(VersionStateUnsupported) != ConnExtensionUnsupported ||
		VersionStateReason(VersionStateLatest) != "" {
		t.Errorf("VersionStateReason mapping broken")
	}
}
