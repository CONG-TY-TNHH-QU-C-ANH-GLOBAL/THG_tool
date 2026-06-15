package leadingest

// Explicit direct-post intake override (hotfix fix/direct-post-intake-filter-bypass).
// When a user issues an explicit "comment this post" command, they have already chosen
// the post as a lead candidate — the generic market-signal filter must NOT veto lead
// creation for it. Deps.ForceLead carries that intent into the shared ingest pipeline.

// ForcedLeadCategory is the lead category assigned when an explicit direct-post intake
// overrides a market-signal veto that would otherwise have rejected the post. The user
// explicitly selected this post, so it enters the workspace as a normal (warm) lead
// rather than being dropped. A post that already scored hot/warm keeps its own category
// (overrideVeto is only reached on a veto).
const ForcedLeadCategory = "warm"

// overrideVeto downgrades a market-signal veto to annotation-only for EXPLICIT
// direct-post intake (Deps.ForceLead). The filter verdict is recorded on the lead
// (Signals: market_filter_result / filter_override_applied / explicit_user_requested)
// for observability, but the lead is still created. Returns true when the veto is
// overridden (the caller must fall through to insert instead of returning); false for a
// normal crawl, where the veto stands unchanged.
//
// Scope: ForceLead is set ONLY by the direct-post intake path (one explicit user-chosen
// post), never by broad crawls — so normal market-signal filtering is byte-for-byte
// unchanged. It does not weaken the P1.1 identity guard: identity matching happens later
// in the poller against the stored lead, independent of this filter decision.
func (d Deps) overrideVeto(out *Outcome, verdict string) bool {
	if !d.ForceLead {
		return false
	}
	out.Signals = append(out.Signals,
		"market_filter_result:"+verdict,
		"filter_override_applied:true",
		"explicit_user_requested:true")
	out.Skipped = ""
	if out.Category == "rejected" || out.Category == "" {
		out.Category = ForcedLeadCategory
	}
	return true
}
