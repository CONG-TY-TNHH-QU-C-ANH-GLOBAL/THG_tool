package crawlcampaign

import (
	"testing"
	"time"
)

func TestEvaluateFreshness(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-24 * time.Hour)
	at := func(d time.Duration) *time.Time { ts := now.Add(d); return &ts }
	// exact carries one instant: posted_at == earliest_utc == latest_utc.
	exact := func(d time.Duration) TimestampParse {
		p, e, l := now.Add(d), now.Add(d), now.Add(d)
		return TimestampParse{Confidence: ConfidenceExact, PostedAt: &p, EarliestUTC: &e, LatestUTC: &l}
	}
	derived := func(earliest, latest time.Duration) TimestampParse {
		e, l := now.Add(earliest), now.Add(latest)
		return TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: &e, LatestUTC: &l}
	}

	rawUnitCase := exact(-1 * time.Hour)
	rawUnitCase.RawUnit = RawUnitDate
	versionCase := exact(-1 * time.Hour)
	versionCase.ParserVersion = "parser-vX"

	cases := []struct {
		name  string
		parse TimestampParse
		want  FreshnessDecision
	}{
		// exact
		{"exact at cutoff", exact(-24 * time.Hour), FreshnessEligibleExact},
		{"exact after cutoff", exact(-1 * time.Hour), FreshnessEligibleExact},
		{"exact before cutoff", exact(-25 * time.Hour), FreshnessStaleExact},
		{"exact at now", exact(0), FreshnessEligibleExact},
		{"exact after now", exact(1 * time.Hour), FreshnessFutureTimestamp},
		{"exact missing bounds", TimestampParse{Confidence: ConfidenceExact, PostedAt: at(-1 * time.Hour)}, FreshnessMalformedTimestamp},
		{"exact inconsistent instants", TimestampParse{Confidence: ConfidenceExact, PostedAt: at(-1 * time.Hour), EarliestUTC: at(-2 * time.Hour), LatestUTC: at(-1 * time.Hour)}, FreshnessMalformedTimestamp},

		// derived_relative
		{"derived earliest at cutoff", derived(-24*time.Hour, -23*time.Hour), FreshnessEligibleDerivedRelative},
		{"derived earliest after cutoff", derived(-23*time.Hour, -22*time.Hour), FreshnessEligibleDerivedRelative},
		{"derived earliest before cutoff", derived(-25*time.Hour, -23*time.Hour), FreshnessStaleDerivedRelative},
		{"derived latest after now", derived(-1*time.Hour, 1*time.Hour), FreshnessFutureTimestamp},
		{"derived missing earliest", TimestampParse{Confidence: ConfidenceDerivedRelative, LatestUTC: at(-1 * time.Hour)}, FreshnessMalformedTimestamp},
		{"derived missing latest", TimestampParse{Confidence: ConfidenceDerivedRelative, EarliestUTC: at(-1 * time.Hour)}, FreshnessMalformedTimestamp},
		{"derived earliest after latest", derived(-22*time.Hour, -23*time.Hour), FreshnessMalformedTimestamp},
		{"derived with posted_at set", TimestampParse{Confidence: ConfidenceDerivedRelative, PostedAt: at(-1 * time.Hour), EarliestUTC: at(-2 * time.Hour), LatestUTC: at(-1 * time.Hour)}, FreshnessMalformedTimestamp},
		{"just-now interval", derived(-1*time.Minute, 0), FreshnessEligibleDerivedRelative},

		// ambiguous / unknown — incidental timestamps must not make them eligible
		{"ambiguous bare", TimestampParse{Confidence: ConfidenceAmbiguous}, FreshnessAmbiguousTimestamp},
		{"ambiguous with incidental timestamps", TimestampParse{Confidence: ConfidenceAmbiguous, PostedAt: at(-1 * time.Hour), EarliestUTC: at(-1 * time.Hour), LatestUTC: at(-1 * time.Hour)}, FreshnessAmbiguousTimestamp},
		{"unknown bare", TimestampParse{Confidence: ConfidenceUnknown}, FreshnessUnknownTimestamp},
		{"unknown with incidental timestamps", TimestampParse{Confidence: ConfidenceUnknown, PostedAt: at(-1 * time.Hour)}, FreshnessUnknownTimestamp},

		// unsupported confidence
		{"unsupported confidence", TimestampParse{Confidence: Confidence("weird")}, FreshnessUnsupportedConfidence},

		// telemetry-only fields do not change the decision
		{"raw_unit does not affect decision", rawUnitCase, FreshnessEligibleExact},
		{"parser_version does not affect decision", versionCase, FreshnessEligibleExact},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateFreshness(tc.parse, cutoff, now); got != tc.want {
				t.Errorf("decision = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFreshnessDecisionEligible(t *testing.T) {
	for _, d := range []FreshnessDecision{FreshnessEligibleExact, FreshnessEligibleDerivedRelative} {
		if !d.Eligible() {
			t.Errorf("%q must be eligible", d)
		}
	}
	ineligible := []FreshnessDecision{
		FreshnessStaleExact, FreshnessStaleDerivedRelative, FreshnessAmbiguousTimestamp,
		FreshnessUnknownTimestamp, FreshnessMalformedTimestamp, FreshnessFutureTimestamp,
		FreshnessUnsupportedConfidence, FreshnessDecision(""),
	}
	for _, d := range ineligible {
		if d.Eligible() {
			t.Errorf("%q must not be eligible", d)
		}
	}
}
