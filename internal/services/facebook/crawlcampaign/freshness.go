package crawlcampaign

import "time"

// FreshnessDecision is the pure verdict of the §4 confidence-specific freshness
// gate for one parsed post timestamp. Reason carries the typed exclusion code
// when Eligible is false, and is empty when Eligible is true.
type FreshnessDecision struct {
	Eligible bool
	Reason   ExitReasonCode
}

// EvaluateFreshness applies the canonical fresh-lead gate (spec §4). freshCutoff
// and now are server-authoritative (never the browser clock). A post is eligible
// only when its worst-case age is provably inside the fresh window: exact
// compares posted_at, derived_relative compares the oldest possible bound, and
// ambiguous/unknown/future/inconsistent parses are excluded with a typed reason.
func EvaluateFreshness(p TimestampParse, freshCutoff, now time.Time) FreshnessDecision {
	switch p.Confidence {
	case ConfidenceExact:
		if p.PostedAt == nil || p.PostedAt.After(now) {
			return FreshnessDecision{Reason: ReasonTimestampInvalid}
		}
		return freshnessByBound(*p.PostedAt, freshCutoff)
	case ConfidenceDerivedRelative:
		if p.EarliestUTC == nil || p.LatestUTC == nil ||
			p.EarliestUTC.After(*p.LatestUTC) ||
			(p.PostedAt != nil && p.PostedAt.After(now)) {
			return FreshnessDecision{Reason: ReasonTimestampInvalid}
		}
		// Judge the worst case: the oldest possible time must still be fresh —
		// provably fresh, not the plausibly-fresh representative posted_at.
		return freshnessByBound(*p.EarliestUTC, freshCutoff)
	case ConfidenceAmbiguous:
		return FreshnessDecision{Reason: ReasonTimestampAmbiguous}
	default:
		return FreshnessDecision{Reason: ReasonTimestampUnparsed}
	}
}

// freshnessByBound is eligible when the judged bound is at or after the cutoff;
// equality is fresh (boundary is inclusive).
func freshnessByBound(bound, freshCutoff time.Time) FreshnessDecision {
	if bound.Before(freshCutoff) {
		return FreshnessDecision{Reason: ReasonStalePost}
	}
	return FreshnessDecision{Eligible: true}
}
