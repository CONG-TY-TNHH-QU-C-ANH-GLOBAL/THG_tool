package rrf

import (
	"fmt"
	"sort"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// fusedEntry holds an asset's rank and score from each upstream
// searcher. A zero rank means the asset was absent from that searcher's
// results.
type fusedEntry struct {
	asset        *assets.Asset
	lexRank      int     // 0 = not in lexical results
	semRank      int     // 0 = not in semantic results
	lexScore     float64 // primary scoring source (for breakdown preservation)
	semScore     float64
	lexBreakdown retrieval.ScoreBreakdown
}

// scoredEntry pairs a fused entry with its computed RRF score.
type scoredEntry struct {
	entry    *fusedEntry
	rrfScore float64
}

// fuseRankings unions the two upstream rankings into one map keyed by
// asset ID, recording each asset's per-searcher rank. The governance
// gate double-checks that banned_claim / hidden assets never enter
// fusion output, incrementing the trace's rejection counters.
func fuseRankings(lexHits, semHits []retrieval.Hit, lexTrace retrieval.Trace, trace *retrieval.Trace) map[int64]*fusedEntry {
	fused := map[int64]*fusedEntry{}

	// Defence-in-depth governance gate. Goal directive G6 makes RRF
	// the FINAL authority: banned_claim and hidden-state assets must
	// NEVER appear in fusion output, even if a future upstream-
	// searcher bug let one through. Upstream searchers already do
	// this filtering; we double-gate here so the contract holds
	// regardless of upstream behaviour. Cost is O(n) per call.
	skipFn := func(a *assets.Asset) bool {
		if a == nil {
			return true
		}
		if a.Type == assets.AssetBannedClaim {
			trace.TotalByReason[retrieval.RejectGovernance]++
			return true
		}
		if a.State == assets.StateHidden {
			trace.TotalByReason[retrieval.RejectStateFilter]++
			return true
		}
		return false
	}

	for i, h := range lexHits {
		if skipFn(h.Asset) {
			continue
		}
		fused[h.Asset.ID] = &fusedEntry{
			asset:    h.Asset,
			lexRank:  i + 1, // 1-indexed for the trace's "rank" semantics
			lexScore: h.Score,
		}
		// Pull breakdown from lex trace if available.
		for _, sh := range lexTrace.Selected {
			if sh.AssetID == h.Asset.ID {
				fused[h.Asset.ID].lexBreakdown = sh.Breakdown
				break
			}
		}
	}
	for i, h := range semHits {
		if skipFn(h.Asset) {
			continue
		}
		entry, ok := fused[h.Asset.ID]
		if !ok {
			entry = &fusedEntry{asset: h.Asset}
			fused[h.Asset.ID] = entry
		}
		entry.semRank = i + 1
		entry.semScore = h.Score
	}
	return fused
}

// rankByRRF computes the RRF score for each fused entry and returns
// them sorted by descending score (stable, so equal scores preserve
// insertion order).
func rankByRRF(fused map[int64]*fusedEntry, rrfK int) []scoredEntry {
	allScored := make([]scoredEntry, 0, len(fused))
	for _, entry := range fused {
		score := 0.0
		if entry.lexRank > 0 {
			score += 1.0 / float64(rrfK+entry.lexRank)
		}
		if entry.semRank > 0 {
			score += 1.0 / float64(rrfK+entry.semRank)
		}
		allScored = append(allScored, scoredEntry{entry, score})
	}
	sort.SliceStable(allScored, func(i, j int) bool {
		return allScored[i].rrfScore > allScored[j].rrfScore
	})
	return allScored
}

// selectAndBuild caps the ranking to k, records the overflow as
// RejectTopKCap rejections, and builds the output hits + Selected
// trace entries for the kept set.
func selectAndBuild(allScored []scoredEntry, k int, trace *retrieval.Trace) []retrieval.Hit {
	keep := allScored
	if len(keep) > k {
		for _, sc := range allScored[k:] {
			retrieval.RecordRejection(trace, sc.entry.asset, retrieval.RejectTopKCap, sc.rrfScore)
		}
		keep = allScored[:k]
	}

	outHits := make([]retrieval.Hit, 0, len(keep))
	trace.Selected = make([]retrieval.ScoredHit, 0, len(keep))
	for _, sc := range keep {
		// Compose the final hit using lex breakdown as base + RRF score
		// as the primary ordering signal. Reason describes the fusion:
		// "rrf bm25=#2 sem=#5" so operators see ranks at a glance.
		reason := fmt.Sprintf("rrf bm25=#%d sem=#%d score=%.4f", sc.entry.lexRank, sc.entry.semRank, sc.rrfScore)
		outHits = append(outHits, retrieval.Hit{
			Asset:  sc.entry.asset,
			Score:  sc.rrfScore,
			Reason: reason,
		})
		trace.Selected = append(trace.Selected, retrieval.ScoredHit{
			AssetID:      sc.entry.asset.ID,
			Title:        sc.entry.asset.Title,
			Type:         sc.entry.asset.Type,
			Score:        sc.rrfScore,
			Breakdown:    sc.entry.lexBreakdown, // preserves explainability of lex side
			Reason:       reason,
			BM25Rank:     sc.entry.lexRank,
			SemanticRank: sc.entry.semRank,
			RRFScore:     sc.rrfScore,
		})
	}
	return outHits
}
