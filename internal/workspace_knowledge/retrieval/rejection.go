package retrieval

import "github.com/thg/scraper/internal/workspace_knowledge/assets"

// RecordRejection adds one entry to the trace's Rejected list AND
// bumps TotalByReason. Capacity is bounded so a 10k-row catalog
// with all rejections doesn't blow up the events table — only the
// first sampleCapPerReason of each reason embed in Rejected[],
// while TotalByReason keeps the uncapped histogram.
//
// Lives beside the Trace types (trace.go) as a same-package sibling.
const SampleCapPerReason = 5

func RecordRejection(t *Trace, a *assets.Asset, reason RejectionReason, score float64) {
	if t == nil || a == nil {
		return
	}
	if t.TotalByReason == nil {
		t.TotalByReason = map[RejectionReason]int{}
	}
	t.TotalByReason[reason]++
	if t.TotalByReason[reason] > SampleCapPerReason {
		return
	}
	t.Rejected = append(t.Rejected, RejectedCandidate{
		AssetID: a.ID,
		Title:   a.Title,
		Type:    a.Type,
		Reason:  reason,
		Score:   score,
	})
}
