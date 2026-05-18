// Package rrf implements Reciprocal Rank Fusion over two Searchers:
// a lexical scorer (hybrid: token + phrase + recency + boost + pin)
// and a semantic scorer (pgvector: cosine similarity).
//
// RRF formula:
//
//   rrf_score(asset) = sum over searchers s of 1 / (k + rank_s(asset))
//
// where k is a smoothing constant (60 is the original Cormack et al.
// 2009 recommendation; pgvector docs use the same default).
//
// Why RRF instead of linear weighting:
//
//   - score ranges DIFFER across searchers. Hybrid produces scores
//     in [0, 1] derived from token overlap; pgvector produces cosine
//     similarities also in [0, 1] but with VERY different distribution.
//     0.5 means "weakly relevant" in one and "garbage" in the other.
//     Linear `0.5*a + 0.5*b` is incomparable.
//   - RRF operates on RANKS only — robust to score scale, well-studied
//     in IR literature, used by Elasticsearch hybrid, Vespa, Vertex AI.
//
// Pipeline order (goal directive PR-3 §3):
//
//   metadata filter → BM25/lexical + vector ANN (in parallel)
//      → RRF fusion → governance filter → budget → assembly
//
// Governance and budget happen AFTER fusion. RRF does not bypass
// either — banned_claim assets dropped by the lexical scorer (hybrid
// has governance built in) simply don't appear in its ranking; the
// fusion's union-of-rankings preserves this exclusion.
//
// Operator intent vs. semantic (§4): pinned assets MUST survive weak
// semantic. The hybrid searcher gives pinned assets a 0.20 score
// contribution that surfaces them in the top results even with zero
// lexical match — so they ALWAYS appear in hybrid's ranking. RRF
// then ensures their score from the lexical side floors the fused
// score. A semantic miss can only push them DOWN within the fusion,
// not OUT.
package rrf

import (
	"context"
	"fmt"
	"sort"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// SearcherImpl identifies the fusion searcher in trace events.
const SearcherImpl = "rrf-v1"

// DefaultK is the RRF smoothing constant. 60 is the standard.
const DefaultK = 60

// DefaultMinConfidence is the floor below which we consider all
// signals weak and the runtime should ask for clarification rather
// than hallucinate (goal directive PR-3 §7). Tuned empirically — fused
// scores below 0.005 mean "no signal scored above the 60-rank tail".
const DefaultMinConfidence = 0.005

// Searcher fuses two upstream Searchers via RRF.
type Searcher struct {
	Lexical  retrieval.Searcher
	Semantic retrieval.Searcher
	// K is the RRF smoothing constant. Defaults to DefaultK when zero.
	K int
	// PoolSize is the per-searcher candidate count requested before
	// fusion. Larger pool = better recall at the cost of latency.
	// Default 50 — covers k=10 final results with 5x headroom.
	PoolSize int
	// MinConfidence: when ALL fused scores are below this, no hits
	// are returned — the runtime asks for clarification rather than
	// surfacing weak matches. Zero means accept any positive score.
	MinConfidence float64
}

// New constructs an RRF Searcher. Either upstream can be nil:
//   - nil lexical → semantic-only (degraded — but RRF still works as a wrapper)
//   - nil semantic → lexical-only (the "no vector capability" SQLite path)
//   - both nil → constructor returns nil → caller must check before use
func New(lexical, semantic retrieval.Searcher) *Searcher {
	if lexical == nil && semantic == nil {
		return nil
	}
	return &Searcher{
		Lexical:       lexical,
		Semantic:      semantic,
		K:             DefaultK,
		PoolSize:      50,
		MinConfidence: DefaultMinConfidence,
	}
}

// TopK satisfies retrieval.Searcher.
func (s *Searcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := s.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}

// TopKWithTrace fuses the two upstream rankings and returns the
// top-k by RRF score, with both per-searcher ranks recorded in the
// trace for explainability.
//
// Important: the upstream Searchers do their own TENANT ISOLATION
// (orgID filter), THEIR OWN GOVERNANCE filtering (banned_claim
// dropped at hybrid; semantic threshold at pgvector), and their own
// BUDGET conventions (TopKCap). RRF inherits all of those — it does
// NOT bypass any layer.
func (s *Searcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	trace := retrieval.Trace{
		Query:         retrieval.TruncateQuery(query),
		SearcherImpl:  SearcherImpl,
		TotalByReason: map[retrieval.RejectionReason]int{},
	}
	if k <= 0 {
		return nil, trace, nil
	}
	if s == nil {
		return nil, trace, fmt.Errorf("rrf searcher: not initialised")
	}
	rrfK := s.K
	if rrfK <= 0 {
		rrfK = DefaultK
	}
	poolSize := s.PoolSize
	if poolSize <= 0 {
		poolSize = 50
	}

	// Call upstreams in parallel-safe sequence. We don't actually
	// run them concurrently because (a) Go's HTTP / DB drivers
	// already pool connections, so two sequential calls share the
	// same connection pool, and (b) parallel adds complexity around
	// trace merging that isn't worth the latency saving at this
	// scale. Both upstreams typically complete in <100ms.
	var lexHits []retrieval.Hit
	var lexTrace retrieval.Trace
	var lexErr error
	if s.Lexical != nil {
		lexHits, lexTrace, lexErr = s.Lexical.TopKWithTrace(ctx, orgID, query, filter, poolSize)
	}

	var semHits []retrieval.Hit
	var semErr error
	if s.Semantic != nil {
		// semTrace intentionally discarded here — the RRF trace is what
		// surfaces to the Operator Replay UI. Per-upstream traces would
		// inflate the events table without adding decision-grade
		// information (the breakdown.Semantic field already carries
		// per-hit semantic provenance).
		semHits, _, semErr = s.Semantic.TopKWithTrace(ctx, orgID, query, filter, poolSize)
	}

	// If both upstreams errored, surface the lexical error (it's the
	// authoritative path — semantic is the enrichment). If only one
	// errored, proceed with the other's results — fusion gracefully
	// degrades to single-source ranking.
	if lexErr != nil && semErr != nil {
		return nil, trace, fmt.Errorf("rrf: both upstreams failed (lex=%v, sem=%v)", lexErr, semErr)
	}

	trace.CandidatesConsidered = len(lexHits) + len(semHits)

	// Build asset → (lex_rank, sem_rank, asset) map.
	type fusedEntry struct {
		asset       *assets.Asset
		lexRank     int     // 0 = not in lexical results
		semRank     int     // 0 = not in semantic results
		lexScore    float64 // primary scoring source (for breakdown preservation)
		semScore    float64
		lexBreakdown retrieval.ScoreBreakdown
	}
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

	// Compute RRF score per entry.
	type scored struct {
		entry    *fusedEntry
		rrfScore float64
	}
	allScored := make([]scored, 0, len(fused))
	for _, entry := range fused {
		score := 0.0
		if entry.lexRank > 0 {
			score += 1.0 / float64(rrfK+entry.lexRank)
		}
		if entry.semRank > 0 {
			score += 1.0 / float64(rrfK+entry.semRank)
		}
		allScored = append(allScored, scored{entry, score})
	}
	sort.SliceStable(allScored, func(i, j int) bool {
		return allScored[i].rrfScore > allScored[j].rrfScore
	})

	// Low-confidence check (§7). If top score is below threshold,
	// return empty so the orchestrator can ask for clarification.
	if len(allScored) > 0 && allScored[0].rrfScore < s.MinConfidence {
		// We still emit a trace so dashboards see "0 hits because
		// low confidence" rather than mysterious empty.
		trace.TotalByReason["low_confidence"] = len(allScored)
		return nil, trace, nil
	}

	// Cap to k, record overflow.
	keep := allScored
	if len(keep) > k {
		for _, sc := range allScored[k:] {
			retrieval.RecordRejection(&trace, sc.entry.asset, retrieval.RejectTopKCap, sc.rrfScore)
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
	return outHits, trace, nil
}

// SearcherName satisfies the fallback wrapper's optional naming hook
// in case RRF gets wrapped further (e.g. for production deployments
// that want timeout-bounded RRF).
func (s *Searcher) SearcherName() string { return SearcherImpl }
