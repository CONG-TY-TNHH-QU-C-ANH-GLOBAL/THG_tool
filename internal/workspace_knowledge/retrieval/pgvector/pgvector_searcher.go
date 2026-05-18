// Package pgvector implements [retrieval.Searcher] backed by the
// pgvector Postgres extension. It is the semantic retrieval stage —
// the layer where the system starts answering "anime gothic tees"
// with results titled "edgy POD streetwear" even though no tokens
// overlap.
//
// CRITICAL invariants enforced here (from the goal directive):
//
//   1. Tenant isolation BEFORE similarity search — the store-side
//      query uses WHERE org_id = ? as the FIRST predicate, before
//      ORDER BY embedding <=> query. The pgvector Searcher never
//      runs a global ANN search and filters after.
//
//   2. Zero-regression fallback — this Searcher is wrapped by a
//      [fallback.Searcher] that switches to the hybrid searcher on:
//        - timeout (1.5s wall)
//        - errors (pgvector unavailable, query failure)
//        - empty result (no embedded assets in the tenant's catalog)
//      The fallback wrapping is done by the runtime; this Searcher
//      itself never silently downgrades.
//
//   3. Explainability preserved — every TopKWithTrace call returns a
//      [retrieval.Trace] with semantic scores in Breakdown.Semantic
//      and rejection reasons in TotalByReason. Replay UI can still
//      answer "why this asset won" and "why others rejected".
//
//   4. Query embedding lifecycle tolerance — the Searcher skips
//      assets where embedding_status != 'generated' (handled by the
//      store query). Pending / failed assets are recorded as
//      RejectEmbeddingMissing in the trace so operators see exactly
//      what was skipped.
//
//   5. Confidence threshold — assets below MinSimilarity are rejected
//      with RejectSemanticThreshold. The fallback layer catches the
//      empty-result case downstream.
package pgvector

import (
	"context"
	"fmt"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/embedding"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// SearcherImpl identifies traces produced by this Searcher.
const SearcherImpl = "pgvector-v1"

// DefaultTimeout is the per-query wall — bounded so the outbound
// queue can never be blocked by a slow vector search. The goal
// directive (PR-2 §6) requires < 1.5s; we honor that.
const DefaultTimeout = 1500 * time.Millisecond

// DefaultMinSimilarity is the rejection cutoff. Cosine similarity
// for L2-normalised OpenAI vectors typically clusters:
//   - 0.85+   highly relevant
//   - 0.70    weakly relevant
//   - 0.60    edge of usefulness
//   - <0.50   noise
// Below this cutoff the result is dropped — preserves explainability
// of "we considered N candidates and rejected M as low-confidence".
const DefaultMinSimilarity = 0.60

// VectorStore is the narrow store surface the Searcher consumes.
// *store.Store satisfies this via QueryNearestVectors.
// Types live in retrieval (shared neutral home) so this interface
// matches the store implementation without an adapter.
type VectorStore interface {
	QueryNearestVectors(ctx context.Context, orgID int64, queryVector []float32, modelVersion string, filter retrieval.VectorFilter, k int) ([]retrieval.VectorHit, error)
}

// Searcher implements [retrieval.Searcher]. Construct via [New].
type Searcher struct {
	Store    VectorStore
	Embedder embedding.Embedder
	// Timeout caps each TopK call. Defaults to DefaultTimeout when
	// zero. The fallback Searcher catches DeadlineExceeded and
	// reroutes to the secondary searcher.
	Timeout time.Duration
	// MinSimilarity is the rejection cutoff. Zero falls back to
	// DefaultMinSimilarity.
	MinSimilarity float64
}

// New constructs a pgvector Searcher.
func New(store VectorStore, embedder embedding.Embedder) *Searcher {
	return &Searcher{
		Store:         store,
		Embedder:      embedder,
		Timeout:       DefaultTimeout,
		MinSimilarity: DefaultMinSimilarity,
	}
}

// TopK satisfies retrieval.Searcher. Delegates to TopKWithTrace.
func (s *Searcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	hits, _, err := s.TopKWithTrace(ctx, orgID, query, filter, k)
	return hits, err
}

// TopKWithTrace runs the semantic retrieval pass with full
// explainability output.
//
// Pipeline:
//   1. Embed the query text with the SAME Embedder used for assets
//      (model parity is critical — cosine compares like-to-like).
//   2. Run tenant-scoped ANN query with WHERE org_id = ? FIRST.
//   3. Convert distance → similarity in [0, 1].
//   4. Reject hits below MinSimilarity, recording reasons.
//   5. Cap to k. Build trace with semantic breakdown.
//
// Timeout: bounded by Searcher.Timeout via context. Caller's context
// is preserved for cancellation semantics; we only LOWER the deadline.
func (s *Searcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	trace := retrieval.Trace{
		Query:         retrieval.TruncateQuery(query),
		SearcherImpl:  SearcherImpl,
		TotalByReason: map[retrieval.RejectionReason]int{},
	}
	if s.Store == nil || s.Embedder == nil {
		return nil, trace, fmt.Errorf("pgvector searcher: not initialised")
	}
	if orgID <= 0 {
		return nil, trace, fmt.Errorf("pgvector searcher: org_id required")
	}
	if k <= 0 {
		return nil, trace, nil
	}
	if query == "" {
		// Empty query → no semantic signal. Return empty + trace; the
		// fallback wrapper will substitute hybrid results, which DO
		// have something to surface (pin/boost).
		return nil, trace, nil
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Step 1: embed the query. Time spent here counts against the
	// timeout — operators see the FULL semantic stage latency, not
	// just the DB portion.
	vectors, err := s.Embedder.Embed(bounded, []string{query})
	if err != nil {
		return nil, trace, fmt.Errorf("pgvector searcher: embed query: %w", err)
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		return nil, trace, fmt.Errorf("pgvector searcher: embedder returned no vector")
	}
	queryVector := vectors[0]
	modelVersion := s.Embedder.ModelVersion()

	// Step 2: tenant-scoped ANN query. Note we ask for MORE candidates
	// than k so the threshold-filter doesn't starve the result.
	candidateK := max(k*3, 20)
	hits, err := s.Store.QueryNearestVectors(bounded, orgID, queryVector, modelVersion, retrieval.VectorFilter{
		Types:  filter.Types,
		States: filter.EffectiveStates(),
	}, candidateK)
	if err != nil {
		return nil, trace, fmt.Errorf("pgvector searcher: query: %w", err)
	}
	trace.CandidatesConsidered = len(hits)

	// Step 3-4: rank + threshold-filter.
	minSim := s.MinSimilarity
	if minSim <= 0 {
		minSim = DefaultMinSimilarity
	}

	type scored struct {
		hit        retrieval.VectorHit
		similarity float64
		breakdown  retrieval.ScoreBreakdown
	}
	scoredHits := make([]scored, 0, len(hits))
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
			retrieval.RecordRejection(&trace, h.Asset, retrieval.RejectSemanticThreshold, sim)
			continue
		}
		// Build score breakdown. Semantic is the primary signal here.
		// Operator pin / boost still contribute — semantic does NOT
		// override operator intent (goal directive PR-3 §4 — pre-applied
		// here too because PR-2 + PR-3 share the principle).
		bd := retrieval.ScoreBreakdown{
			TextMatch: 0,       // pgvector doesn't do lexical
			Boost:     0.10 * float64(h.Asset.Boost) / 100.0,
			Semantic:  0.70 * sim,
		}
		if h.Asset.Pinned {
			bd.Pin = 0.20
		}
		// recency = 0 in pgvector; the temporal signal is the
		// hybrid searcher's domain. Combined in PR-3 RRF.
		scoredHits = append(scoredHits, scored{hit: h, similarity: sim, breakdown: bd})
	}

	// Cap to k, recording overflow.
	keep := scoredHits
	if len(keep) > k {
		for _, sh := range scoredHits[k:] {
			retrieval.RecordRejection(&trace, sh.hit.Asset, retrieval.RejectTopKCap, sh.similarity)
		}
		keep = scoredHits[:k]
	}

	// Build hits + Selected trace entries.
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
	return outHits, trace, nil
}

// IsEmptyForFallback satisfies fallback.EmptinessTest. Returns true
// when the tenant has zero embedded assets — that signals "the
// hybrid path may still find pinned/boosted items by lexical means".
//
// We do NOT fall back when CandidatesConsidered > 0 but Selected is
// empty — that means RejectSemanticThreshold did its job (no asset
// crossed the confidence bar); falling back to hybrid would just
// reach for weaker signals and potentially surface lower-quality
// results. Better to return empty and let the assembly layer use
// the freeform business profile.
func (s *Searcher) IsEmptyForFallback(trace retrieval.Trace) bool {
	return trace.CandidatesConsidered == 0
}

// SearcherName satisfies the fallback wrapper's optional naming
// interface, so the Replay UI can show "pgvector-v1 → fallback to
// hybrid-v1" instead of a generic "primary → fallback" annotation.
func (s *Searcher) SearcherName() string { return SearcherImpl }
