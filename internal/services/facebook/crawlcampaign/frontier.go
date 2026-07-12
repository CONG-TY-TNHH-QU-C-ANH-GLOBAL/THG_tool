package crawlcampaign

// FrontierStaleStreak is the number of consecutive confidently-stale posts that
// ends a run at the temporal frontier (spec §5). Conservative default; tuned
// only later under telemetry evidence.
const FrontierStaleStreak = 8

// NextStaleStreak advances the temporal-frontier streak for one post. Only
// confident (exact/derived_relative) stale evidence extends the streak; a fresh
// confident post or any ambiguous/unknown post resets it — the streak is
// consecutive proof of staleness, and absence of proof breaks the chain.
func NextStaleStreak(current int, confidence Confidence, fresh bool) int {
	if !confidence.IsConfident() || fresh {
		return 0
	}
	return current + 1
}

// FrontierReached reports whether the stale streak has hit the stop threshold.
func FrontierReached(staleStreak int) bool {
	return staleStreak >= FrontierStaleStreak
}
