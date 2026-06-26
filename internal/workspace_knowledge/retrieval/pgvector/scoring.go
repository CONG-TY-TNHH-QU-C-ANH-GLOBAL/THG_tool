package pgvector

import (
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// scoredVectorHit pairs a vector hit with its derived similarity and
// score breakdown, ready for capping and output assembly.
type scoredVectorHit struct {
	hit        retrieval.VectorHit
	similarity float64
	breakdown  retrieval.ScoreBreakdown
}

// scoreVectorHits converts each hit's cosine distance to a clamped
// similarity, drops anything below minSim (recording the rejection),
// and builds the semantic-primary score breakdown for survivors.
func scoreVectorHits(hits []retrieval.VectorHit, minSim float64, trace *retrieval.Trace) []scoredVectorHit {
	scoredHits := make([]scoredVectorHit, 0, len(hits))
	for _, h := range hits {
		// Cosine distance → similarity. For L2-normalised vectors
		// (which OpenAI's embeddings are), distance in [0, 2] maps
		// to similarity = 1 - distance, in [-1, 1]. Clamp to [0, 1]
		// for the breakdown's expected range.
		sim := 1.0 - h.Distance
		if sim < 0 {
			sim = 0
		}
		if sim > 1 {
			sim = 1
		}
		if sim < minSim {
			retrieval.RecordRejection(trace, h.Asset, retrieval.RejectSemanticThreshold, sim)
			continue
		}
		// Build score breakdown. Semantic is the primary signal here.
		// Operator pin / boost still contribute — semantic does NOT
		// override operator intent (goal directive PR-3 §4 — pre-applied
		// here too because PR-2 + PR-3 share the principle).
		bd := retrieval.ScoreBreakdown{
			TextMatch: 0, // pgvector doesn't do lexical
			Boost:     0.10 * float64(h.Asset.Boost) / 100.0,
			Semantic:  0.70 * sim,
		}
		if h.Asset.Pinned {
			bd.Pin = 0.20
		}
		// recency = 0 in pgvector; the temporal signal is the
		// hybrid searcher's domain. Combined in PR-3 RRF.
		scoredHits = append(scoredHits, scoredVectorHit{hit: h, similarity: sim, breakdown: bd})
	}
	return scoredHits
}

// buildVectorOutput caps the scored hits to k (recording the overflow
// as RejectTopKCap), then assembles the output hits and Selected trace
// entries for the kept set.
func buildVectorOutput(scoredHits []scoredVectorHit, k int, trace *retrieval.Trace) []retrieval.Hit {
	keep := scoredHits
	if len(keep) > k {
		for _, sh := range scoredHits[k:] {
			retrieval.RecordRejection(trace, sh.hit.Asset, retrieval.RejectTopKCap, sh.similarity)
		}
		keep = scoredHits[:k]
	}

	outHits := make([]retrieval.Hit, 0, len(keep))
	trace.Selected = make([]retrieval.ScoredHit, 0, len(keep))
	for _, sh := range keep {
		finalScore := retrieval.Clamp01(sh.breakdown.TextMatch + sh.breakdown.Boost + sh.breakdown.Pin + sh.breakdown.Semantic + sh.breakdown.Recency)
		reason := retrieval.BuildReason(sh.similarity, sh.hit.Asset.Pinned, sh.hit.Asset.Boost)
		outHits = append(outHits, retrieval.Hit{
			Asset:  sh.hit.Asset,
			Score:  finalScore,
			Reason: reason,
		})
		trace.Selected = append(trace.Selected, retrieval.ScoredHit{
			AssetID:   sh.hit.Asset.ID,
			Title:     sh.hit.Asset.Title,
			Type:      sh.hit.Asset.Type,
			Score:     finalScore,
			Breakdown: sh.breakdown,
			Reason:    reason,
		})
	}
	return outHits
}
