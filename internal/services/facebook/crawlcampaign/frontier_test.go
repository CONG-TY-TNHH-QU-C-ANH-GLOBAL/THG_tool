package crawlcampaign

import "testing"

func TestNextStaleStreak(t *testing.T) {
	cases := []struct {
		name       string
		current    int
		confidence Confidence
		fresh      bool
		want       int
	}{
		{"confident stale increments", 5, ConfidenceExact, false, 6},
		{"derived stale increments", 0, ConfidenceDerivedRelative, false, 1},
		{"confident fresh resets", 5, ConfidenceExact, true, 0},
		{"ambiguous resets even if stale", 5, ConfidenceAmbiguous, false, 0},
		{"unknown resets even if stale", 5, ConfidenceUnknown, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextStaleStreak(tc.current, tc.confidence, tc.fresh); got != tc.want {
				t.Errorf("NextStaleStreak(%d, %q, %v) = %d, want %d", tc.current, tc.confidence, tc.fresh, got, tc.want)
			}
		})
	}
}

func TestFrontierReached(t *testing.T) {
	if FrontierReached(FrontierStaleStreak - 1) {
		t.Error("streak below threshold must not reach the frontier")
	}
	if !FrontierReached(FrontierStaleStreak) {
		t.Error("streak at the threshold must reach the frontier")
	}
}

// One ambiguous post mid-run breaks the consecutive-stale chain, so the frontier
// is not reached even though enough stale posts were seen overall.
func TestFrontierStreak_ResetBreaksChain(t *testing.T) {
	streak := 0
	for range FrontierStaleStreak - 1 {
		streak = NextStaleStreak(streak, ConfidenceExact, false)
	}
	streak = NextStaleStreak(streak, ConfidenceAmbiguous, false) // resets
	streak = NextStaleStreak(streak, ConfidenceExact, false)
	if FrontierReached(streak) {
		t.Fatalf("chain broken by ambiguous post must not reach frontier, streak=%d", streak)
	}
}
