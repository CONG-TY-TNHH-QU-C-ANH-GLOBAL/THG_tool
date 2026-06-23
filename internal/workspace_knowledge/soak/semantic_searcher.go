package soak

import (
	"context"
	"fmt"
	"sort"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// mockSemanticSearcher simulates the pgvector Searcher for soak
// runs against SQLite. It uses the same Embedder + cosine math the
// real pgvector path would — but loads ALL approved assets into
// memory and computes cosine in Go.
//
// At soak-catalog scale (~25 assets) this is fine. The signal we're
// measuring is RETRIEVAL QUALITY + EXPLAINABILITY, not query latency.
// In production-soak (real PG + pgvector), the operator swaps this
// for pgvector.New and re-runs the harness — same Report shape.
//
// Why ship a simulated searcher rather than skip semantic in soak?
// Because the soak must answer "did the RRF fusion correctly use
// both signals?" without that data point the soak doesn't validate
// PR-3 §1 (RRF math) or §6 (both ranks in trace).
type mockSemanticSearcher struct {
	store    semanticStore
	embedder embedding.Embedder
	minSim   float64
}

// semanticStore is the narrow surface this searcher needs.
// *knowledge.Store satisfies it.
type semanticStore interface {
	ListAssetsForOrg(ctx context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error)
}

func newMockSemanticSearcher(s semanticStore, e embedding.Embedder) *mockSemanticSearcher {
	return &mockSemanticSearcher{
		store:    s,
		embedder: e,
		minSim:   0.30, // looser than production pgvector (0.60) because cluster vectors are sparser
	}
}

func (m *mockSemanticSearcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := m.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}

func (m *mockSemanticSearcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	trace := retrieval.Trace{
		Query:         retrieval.TruncateQuery(query),
		SearcherImpl:  "mock-semantic-soak",
		TotalByReason: map[retrieval.RejectionReason]int{},
	}
	if query == "" || k <= 0 {
		return nil, trace, nil
	}

	// Embed query.
	vecs, err := m.embedder.Embed(ctx, []string{query})
	if err != nil || len(vecs) != 1 {
		return nil, trace, fmt.Errorf("mock semantic: embed: %w", err)
	}
	queryVec := vecs[0]

	// Load approved candidates. The real pgvector path filters at
	// the SQL level; here we filter in Go since we're operating on
	// in-memory results.
	candidates, err := m.store.ListAssetsForOrg(ctx, orgID, assets.ListFilter{
		Types:  filter.Types,
		States: filter.EffectiveStates(),
	})
	if err != nil {
		return nil, trace, err
	}
	trace.CandidatesConsidered = len(candidates)

	scoredList := m.scoreCandidates(ctx, queryVec, candidates, &trace)
	sort.SliceStable(scoredList, func(i, j int) bool {
		return scoredList[i].sim > scoredList[j].sim
	})
	keep := capSemanticTopK(scoredList, k, &trace)
	hits, selected := buildSemanticHits(keep)
	trace.Selected = selected
	return hits, trace, nil
}

// scoredAsset pairs a candidate asset with its cosine similarity to the query.
type scoredAsset struct {
	asset *assets.Asset
	sim   float64
}

// scoreCandidates embeds each candidate, drops banned claims (governance) and
// below-threshold matches (recording the rejection reason in trace), and returns the
// survivors. Verbatim extraction of the former inline scoring loop.
func (m *mockSemanticSearcher) scoreCandidates(ctx context.Context, queryVec []float32, candidates []*assets.Asset, trace *retrieval.Trace) []scoredAsset {
	scoredList := make([]scoredAsset, 0, len(candidates))
	for _, a := range candidates {
		// Skip banned claims — governance applies here too. Real
		// pgvector path skips at the SQL filter level; we do the
		// same in-memory.
		if a.Type == assets.AssetBannedClaim {
			retrieval.RecordRejection(trace, a, retrieval.RejectGovernance, 0)
			continue
		}
		// Compute the candidate's embedding on the fly. Real pgvector
		// pre-computes via the worker; this simulation costs O(n) per
		// query but that's fine at soak-catalog scale.
		assetText := embedding.BuildInputText(a)
		assetVecs, err := m.embedder.Embed(ctx, []string{assetText})
		if err != nil || len(assetVecs) != 1 {
			continue
		}
		sim := cosineSimilarity(queryVec, assetVecs[0])
		if sim < m.minSim {
			retrieval.RecordRejection(trace, a, retrieval.RejectSemanticThreshold, sim)
			continue
		}
		scoredList = append(scoredList, scoredAsset{a, sim})
	}
	return scoredList
}

// capSemanticTopK keeps the top-k scored assets, recording the dropped tail's rejection
// reason in trace. Verbatim extraction of the former inline top-k cap.
func capSemanticTopK(scoredList []scoredAsset, k int, trace *retrieval.Trace) []scoredAsset {
	if len(scoredList) <= k {
		return scoredList
	}
	for _, s := range scoredList[k:] {
		retrieval.RecordRejection(trace, s.asset, retrieval.RejectTopKCap, s.sim)
	}
	return scoredList[:k]
}

// buildSemanticHits turns the kept scored assets into hits + trace-selected rows.
// Verbatim extraction of the former inline hit-building loop (same score breakdown).
func buildSemanticHits(keep []scoredAsset) ([]retrieval.Hit, []retrieval.ScoredHit) {
	hits := make([]retrieval.Hit, 0, len(keep))
	selected := make([]retrieval.ScoredHit, 0, len(keep))
	for _, s := range keep {
		bd := retrieval.ScoreBreakdown{
			Semantic: 0.70 * s.sim,
		}
		if s.asset.Pinned {
			bd.Pin = 0.20
		}
		bd.Boost = 0.10 * float64(s.asset.Boost) / 100.0
		score := retrieval.Clamp01(bd.TextMatch + bd.Boost + bd.Pin + bd.Semantic + bd.Recency)
		reason := retrieval.BuildReason(s.sim, s.asset.Pinned, s.asset.Boost)
		hits = append(hits, retrieval.Hit{Asset: s.asset, Score: score, Reason: reason})
		selected = append(selected, retrieval.ScoredHit{
			AssetID:   s.asset.ID,
			Title:     s.asset.Title,
			Type:      s.asset.Type,
			Score:     score,
			Breakdown: bd,
			Reason:    reason,
		})
	}
	return hits, selected
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		magA += float64(a[i]) * float64(a[i])
		magB += float64(b[i]) * float64(b[i])
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (sqrtf(magA) * sqrtf(magB))
}

// sqrtf: avoids importing math just for one call; inline NEGATIVE-safe square.
func sqrtf(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton iterations — 5 is enough for double precision.
	g := x / 2
	for range 5 {
		g = (g + x/g) / 2
	}
	return g
}
