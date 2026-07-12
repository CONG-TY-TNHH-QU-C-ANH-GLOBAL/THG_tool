package crawlcampaign

import "time"

// EvaluateFreshness applies the canonical fresh-lead gate (spec §4), returning
// the complete typed per-item decision. cutoff and now are server-authoritative
// (never the browser clock). A post is eligible only when its worst-case age is
// provably inside the fresh window. Ambiguous and unknown confidences are never
// eligible regardless of any incidental timestamps they carry.
func EvaluateFreshness(timestamp TimestampParse, cutoff time.Time, now time.Time) FreshnessDecision {
	switch timestamp.Confidence {
	case ConfidenceExact:
		return evaluateExact(timestamp, cutoff, now)
	case ConfidenceDerivedRelative:
		return evaluateDerivedRelative(timestamp, cutoff, now)
	case ConfidenceAmbiguous:
		return FreshnessAmbiguousTimestamp
	case ConfidenceUnknown:
		return FreshnessUnknownTimestamp
	default:
		return FreshnessUnsupportedConfidence
	}
}

// evaluateExact requires a single instant: posted_at, earliest_utc, and
// latest_utc must all be present and equal, and judges that instant.
func evaluateExact(timestamp TimestampParse, cutoff, now time.Time) FreshnessDecision {
	switch {
	case timestamp.PostedAt == nil || timestamp.EarliestUTC == nil || timestamp.LatestUTC == nil:
		return FreshnessMalformedTimestamp
	case !timestamp.PostedAt.Equal(*timestamp.EarliestUTC) || !timestamp.PostedAt.Equal(*timestamp.LatestUTC):
		return FreshnessMalformedTimestamp
	case timestamp.PostedAt.After(now):
		return FreshnessFutureTimestamp
	case timestamp.PostedAt.Before(cutoff):
		return FreshnessStaleExact
	default:
		return FreshnessEligibleExact
	}
}

// evaluateDerivedRelative judges the worst case: the oldest possible time
// (earliest_utc) must still be fresh — provably fresh, not plausibly fresh.
// A derived parse carries an interval only; posted_at must be nil.
func evaluateDerivedRelative(timestamp TimestampParse, cutoff, now time.Time) FreshnessDecision {
	switch {
	case timestamp.PostedAt != nil:
		return FreshnessMalformedTimestamp
	case timestamp.EarliestUTC == nil || timestamp.LatestUTC == nil || timestamp.EarliestUTC.After(*timestamp.LatestUTC):
		return FreshnessMalformedTimestamp
	case timestamp.LatestUTC.After(now):
		return FreshnessFutureTimestamp
	case timestamp.EarliestUTC.Before(cutoff):
		return FreshnessStaleDerivedRelative
	default:
		return FreshnessEligibleDerivedRelative
	}
}
