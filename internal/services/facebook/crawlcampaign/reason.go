package crawlcampaign

// FreshnessDecision is the complete typed outcome of the per-item timestamp
// freshness gate (spec §3–§4): eligible or not, exact or derived_relative, and
// the ineligibility cause (stale / ambiguous / unknown / malformed / future /
// unsupported confidence). Distinct from a run's terminal RunExitReason.
type FreshnessDecision string

const (
	FreshnessEligibleExact           FreshnessDecision = "eligible_exact"
	FreshnessEligibleDerivedRelative FreshnessDecision = "eligible_derived_relative"
	FreshnessStaleExact              FreshnessDecision = "stale_exact"
	FreshnessStaleDerivedRelative    FreshnessDecision = "stale_derived_relative"
	FreshnessAmbiguousTimestamp      FreshnessDecision = "ambiguous_timestamp"
	FreshnessUnknownTimestamp        FreshnessDecision = "unknown_timestamp"
	FreshnessMalformedTimestamp      FreshnessDecision = "malformed_timestamp"
	FreshnessFutureTimestamp         FreshnessDecision = "future_timestamp"
	FreshnessUnsupportedConfidence   FreshnessDecision = "unsupported_confidence"
)

// Eligible reports whether the decision is a lead-eligible outcome. The zero
// value and any unknown decision are not eligible.
func (d FreshnessDecision) Eligible() bool {
	return d == FreshnessEligibleExact || d == FreshnessEligibleDerivedRelative
}

// RunExitReason is a typed terminal reason recorded on a run
// (facebook_crawl_runs.exit_reason_code) — distinct from per-item freshness
// decisions. The stop logic that emits these lands in later reviewed slices.
type RunExitReason string

const (
	RunExitFrontierReached         RunExitReason = "frontier_reached"
	RunExitTimestampParserDegraded RunExitReason = "timestamp_parser_degraded"
)
