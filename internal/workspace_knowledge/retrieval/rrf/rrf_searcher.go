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

	fused := fuseRankings(lexHits, semHits, lexTrace, &trace)
	allScored := rankByRRF(fused, rrfK)

	// Low-confidence check (§7). If top score is below threshold,
	// return empty so the orchestrator can ask for clarification.
	if len(allScored) > 0 && allScored[0].rrfScore < s.MinConfidence {
		// We still emit a trace so dashboards see "0 hits because
		// low confidence" rather than mysterious empty.
		trace.TotalByReason["low_confidence"] = len(allScored)
		return nil, trace, nil
	}

	return selectAndBuild(allScored, k, &trace), trace, nil
}

// SearcherName satisfies the fallback wrapper's optional naming hook
// in case RRF gets wrapped further (e.g. for production deployments
// that want timeout-bounded RRF).
func (s *Searcher) SearcherName() string { return SearcherImpl }
