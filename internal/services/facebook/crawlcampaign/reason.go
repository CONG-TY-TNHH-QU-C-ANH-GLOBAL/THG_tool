package crawlcampaign

// ExitReasonCode is a typed reason recorded on a run or a per-item exclusion.
// Values are the canonical wire/DB strings (facebook_crawl_runs.exit_reason_code
// and the ingest-gate exclusion reasons, spec §3–§5).
type ExitReasonCode string

// Per-item freshness-gate exclusions (spec §3–§4).
const (
	ReasonStalePost          ExitReasonCode = "stale_post"
	ReasonTimestampUnparsed  ExitReasonCode = "timestamp_unparsed"
	ReasonTimestampAmbiguous ExitReasonCode = "timestamp_ambiguous"
	ReasonTimestampInvalid   ExitReasonCode = "timestamp_invalid"
	ReasonDuplicateLead      ExitReasonCode = "duplicate_lead"
)

// Run-level terminal reasons (spec §4–§5).
const (
	ReasonFrontierReached         ExitReasonCode = "frontier_reached"
	ReasonTimestampParserDegraded ExitReasonCode = "timestamp_parser_degraded"
)
