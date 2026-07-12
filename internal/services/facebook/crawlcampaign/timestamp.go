package crawlcampaign

import "time"

// Confidence is the parser's certainty about a post's age (spec §4). Only exact
// and derived_relative carry a freshness claim strong enough to be lead-eligible.
type Confidence string

const (
	ConfidenceExact           Confidence = "exact"
	ConfidenceDerivedRelative Confidence = "derived_relative"
	ConfidenceAmbiguous       Confidence = "ambiguous"
	ConfidenceUnknown         Confidence = "unknown"
)

// IsConfident reports whether the confidence carries a usable freshness claim.
func (c Confidence) IsConfident() bool {
	return c == ConfidenceExact || c == ConfidenceDerivedRelative
}

// RawUnit is the typed unit a relative timestamp was derived from. Only this
// typed unit leaves the browser; raw page text never does (PR-C0.5 privacy rule).
type RawUnit string

const (
	RawUnitMinute RawUnit = "minute"
	RawUnitHour   RawUnit = "hour"
	RawUnitDay    RawUnit = "day"
	RawUnitWeek   RawUnit = "week"
	RawUnitDate   RawUnit = "date"
	RawUnitNone   RawUnit = "none"
)

// TimestampParse is the one canonical post-timestamp DTO (spec §4): the parser
// returns it, the extension wire carries it per item verbatim, and the
// server-side freshness gate consumes exactly these fields. Interval bounds are
// UTC; the server clock is authoritative, never the browser.
type TimestampParse struct {
	PostedAt      *time.Time `json:"posted_at"`
	Confidence    Confidence `json:"confidence"`
	EarliestUTC   *time.Time `json:"earliest_utc"`
	LatestUTC     *time.Time `json:"latest_utc"`
	RawUnit       RawUnit    `json:"raw_unit,omitempty"`
	ParserVersion string     `json:"parser_version,omitempty"`
}
