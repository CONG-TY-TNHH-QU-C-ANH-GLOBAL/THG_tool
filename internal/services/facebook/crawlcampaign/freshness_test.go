package crawlcampaign

import (
	"testing"
	"time"
)

func TestEvaluateFreshness(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-24 * time.Hour)
	at := func(d time.Duration) *time.Time { ts := now.Add(d); return &ts }

	cases := []struct {
		name         string
		parse        TimestampParse
		wantEligible bool
		wantReason   ExitReasonCode
	}{
		{"exact fresh", TimestampParse{Confidence: ConfidenceExact, PostedAt: at(-1 * time.Hour)}, true, ""},
		{"exact stale", TimestampParse{Confidence: ConfidenceExact, PostedAt: at(-25 * time.Hour)}, false, ReasonStalePost},
		{"exact boundary is fresh", TimestampParse{Confidence: ConfidenceExact, PostedAt: at(-24 * time.Hour)}, true, ""},
		{"exact future is invalid", TimestampParse{Confidence: ConfidenceExact, PostedAt: at(1 * time.Hour)}, false, ReasonTimestampInvalid},
		{"exact missing posted_at is invalid", TimestampParse{Confidence: ConfidenceExact}, false, ReasonTimestampInvalid},

		// "22 giờ" → age [22h,23h) → earliest = now-23h, inside the 24h window.
		{"derived fresh", TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: at(-23 * time.Hour), LatestUTC: at(-22 * time.Hour)}, true, ""},
		// "23 giờ" → earliest = now-24h = cutoff → fresh at the boundary.
		{"derived boundary is fresh", TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: at(-24 * time.Hour), LatestUTC: at(-23 * time.Hour)}, true, ""},
		// "24 giờ" → earliest = now-25h → worst case crosses the cutoff.
		{"derived stale", TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: at(-25 * time.Hour), LatestUTC: at(-24 * time.Hour)}, false, ReasonStalePost},
		{"derived missing bounds is invalid", TimestampParse{Confidence: ConfidenceDerivedRelative}, false, ReasonTimestampInvalid},
		{"derived inverted interval is invalid", TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: at(-22 * time.Hour), LatestUTC: at(-23 * time.Hour)}, false, ReasonTimestampInvalid},
		{"derived future representative is invalid", TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: at(-1 * time.Hour), LatestUTC: at(1 * time.Hour), PostedAt: at(30 * time.Minute)}, false, ReasonTimestampInvalid},

		{"ambiguous excluded", TimestampParse{Confidence: ConfidenceAmbiguous}, false, ReasonTimestampAmbiguous},
		{"unknown excluded", TimestampParse{Confidence: ConfidenceUnknown}, false, ReasonTimestampUnparsed},
		{"unrecognized confidence excluded as unparsed", TimestampParse{Confidence: Confidence("weird")}, false, ReasonTimestampUnparsed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateFreshness(tc.parse, cutoff, now)
			if got.Eligible != tc.wantEligible {
				t.Errorf("Eligible = %v, want %v", got.Eligible, tc.wantEligible)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}
