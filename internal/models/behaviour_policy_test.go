package models

import (
	"testing"
	"time"
)

// Trust levels must produce monotonically rising daily caps as trust
// increases — this is the policy invariant the orchestrator relies on.
func TestResolveBehaviourCaps_TrustMonotonicity(t *testing.T) {
	order := []TrustLevel{TrustCold, TrustWarming, TrustWarm, TrustTrusted, TrustSacrificial}
	var prev BehaviourCaps
	for i, lvl := range order {
		caps := ResolveBehaviourCaps(lvl, "")
		if i > 0 {
			if caps.CommentsPerDay < prev.CommentsPerDay {
				t.Errorf("comments cap not monotonic at %s: %d < %d", lvl, caps.CommentsPerDay, prev.CommentsPerDay)
			}
			if caps.InboxPerDay < prev.InboxPerDay {
				t.Errorf("inbox cap not monotonic at %s: %d < %d", lvl, caps.InboxPerDay, prev.InboxPerDay)
			}
			// Cooldowns should shrink (or stay equal) as trust rises.
			if caps.SameGroupCooldown > prev.SameGroupCooldown {
				t.Errorf("same-group cooldown not shrinking at %s: %v > %v", lvl, caps.SameGroupCooldown, prev.SameGroupCooldown)
			}
		}
		prev = caps
	}
}

// Unknown trust level falls back to TrustWarming, not zero. Zero would
// make the queue layer block every action.
func TestResolveBehaviourCaps_UnknownTrustFallsBack(t *testing.T) {
	caps := ResolveBehaviourCaps(TrustLevel("nonsense-trust-value"), "")
	warming := ResolveBehaviourCaps(TrustWarming, "")
	if caps != warming {
		t.Errorf("unknown trust must fall back to warming preset, got %+v", caps)
	}
}

// aggressive_outreach multiplies caps, passive_engagement halves them.
// inbox_only zeros out non-inbox caps regardless of multiplier.
func TestResolveBehaviourCaps_RoleMultipliers(t *testing.T) {
	base := ResolveBehaviourCaps(TrustWarm, "")
	agg := ResolveBehaviourCaps(TrustWarm, "aggressive_outreach")
	pas := ResolveBehaviourCaps(TrustWarm, "passive_engagement")
	ib := ResolveBehaviourCaps(TrustWarm, "inbox_only")

	if agg.CommentsPerDay <= base.CommentsPerDay {
		t.Errorf("aggressive_outreach must raise comments cap (base=%d agg=%d)", base.CommentsPerDay, agg.CommentsPerDay)
	}
	if pas.CommentsPerDay >= base.CommentsPerDay {
		t.Errorf("passive_engagement must lower comments cap (base=%d pas=%d)", base.CommentsPerDay, pas.CommentsPerDay)
	}
	if ib.CommentsPerDay != 0 || ib.GroupPostsPerDay != 0 || ib.ProfilePostsPerDay != 0 {
		t.Errorf("inbox_only must zero non-inbox caps, got %+v", ib)
	}
	if ib.InboxPerDay == 0 {
		t.Errorf("inbox_only must keep inbox cap, got %d", ib.InboxPerDay)
	}
}

// OverlayCaps must respect missing-field semantics: only fields present in
// the JSON should overwrite the resolved caps. Empty / invalid JSON
// passes the base caps through unchanged.
func TestOverlayCaps(t *testing.T) {
	base := ResolveBehaviourCaps(TrustWarm, "")

	if got := OverlayCaps(base, ""); got != base {
		t.Errorf("empty override must pass through, got %+v", got)
	}
	if got := OverlayCaps(base, "{}"); got != base {
		t.Errorf("empty json must pass through, got %+v", got)
	}
	if got := OverlayCaps(base, "not-json"); got != base {
		t.Errorf("invalid json must pass through, got %+v", got)
	}

	overlay := `{"comments_per_day": 999}`
	out := OverlayCaps(base, overlay)
	if out.CommentsPerDay != 999 {
		t.Errorf("override should set comments cap to 999, got %d", out.CommentsPerDay)
	}
	if out.InboxPerDay != base.InboxPerDay {
		t.Errorf("non-overridden field changed: inbox %d → %d", base.InboxPerDay, out.InboxPerDay)
	}
}

// SignalWeights covers every defined RiskSignal so the writer can never
// receive a known signal that has no weight.
func TestSignalWeights_CoverAllSignals(t *testing.T) {
	allSignals := []RiskSignal{
		RiskSignalFailure,
		RiskSignalSuccess,
		RiskSignalCaptcha,
		RiskSignalRedirectAnomaly,
		RiskSignalActionRejected,
		RiskSignalReplyReceived,
		RiskSignalBrowserCrash,
		RiskSignalCommentDeleted,
	}
	for _, s := range allSignals {
		if _, ok := SignalWeights[s]; !ok {
			t.Errorf("signal %q has no default weight", s)
		}
	}
}

// CapForAction maps action_type → daily cap. Unknown types must return 0
// so the queue layer reads "no cap from the daily mechanism".
func TestCapForAction(t *testing.T) {
	caps := BehaviourCaps{CommentsPerDay: 1, InboxPerDay: 2, GroupPostsPerDay: 3, ProfilePostsPerDay: 4}
	cases := map[string]int{
		"comment":      1,
		"inbox":        2,
		"group_post":   3,
		"profile_post": 4,
		"unknown":      0,
		"":             0,
	}
	for action, want := range cases {
		if got := caps.CapForAction(action); got != want {
			t.Errorf("CapForAction(%q) = %d, want %d", action, got, want)
		}
	}
}

// CounterForAction is the mirror of CapForAction for the runtime state.
func TestCounterForAction(t *testing.T) {
	r := AccountRuntimeState{CommentsToday: 1, InboxToday: 2, GroupPostsToday: 3, ProfilePostsToday: 4}
	cases := map[string]int{
		"comment":      1,
		"inbox":        2,
		"group_post":   3,
		"profile_post": 4,
		"unknown":      0,
	}
	for action, want := range cases {
		if got := r.CounterForAction(action); got != want {
			t.Errorf("CounterForAction(%q) = %d, want %d", action, got, want)
		}
	}
}

// Sanity check: cooldown values from the trust table are positive (a
// missing default would silently allow burst sending).
func TestTrustCooldownsArePositive(t *testing.T) {
	for _, lvl := range []TrustLevel{TrustCold, TrustWarming, TrustWarm, TrustTrusted, TrustSacrificial} {
		caps := ResolveBehaviourCaps(lvl, "")
		if caps.SameGroupCooldown <= 0 || caps.SamePostCooldown <= 0 || caps.SameProfileCooldown <= 0 {
			t.Errorf("%s: cooldowns must be > 0, got %+v", lvl, caps)
		}
		if caps.GlobalActionCooldown <= 0 {
			t.Errorf("%s: global cooldown must be > 0, got %v", lvl, caps.GlobalActionCooldown)
		}
		// Use time.Duration's String() to make failures readable.
		_ = caps.SameGroupCooldown.String()
		_ = time.Second
	}
}
