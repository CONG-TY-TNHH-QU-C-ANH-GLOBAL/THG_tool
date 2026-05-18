package rrf

import (
	"context"
	"errors"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// scriptedSearcher returns canned hits + trace. Tests construct two
// such searchers to drive RRF deterministically.
type scriptedSearcher struct {
	hits  []retrieval.Hit
	trace retrieval.Trace
	err   error
}

func (s *scriptedSearcher) TopK(ctx context.Context, _ int64, _ string, _ retrieval.SearchFilter, _ int) ([]retrieval.Hit, error) {
	return s.hits, s.err
}
func (s *scriptedSearcher) TopKWithTrace(ctx context.Context, _ int64, _ string, _ retrieval.SearchFilter, _ int) ([]retrieval.Hit, retrieval.Trace, error) {
	return s.hits, s.trace, s.err
}

func mkHit(id int64, title string, score float64) retrieval.Hit {
	return retrieval.Hit{
		Asset: &assets.Asset{
			ID:    id,
			OrgID: 7,
			Type:  assets.AssetPODProduct,
			Title: title,
		},
		Score: score,
	}
}

// Core RRF math: an asset that appears in BOTH rankings scores
// HIGHER than an asset appearing in only ONE.
func TestRRF_DualRankingBeatsSingleRanking(t *testing.T) {
	lex := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(1, "Cat Tee", 0.9),     // lex rank 1
		mkHit(2, "Dog Mug", 0.8),     // lex rank 2 — NOT in semantic
		mkHit(3, "Pet Hoodie", 0.7),  // lex rank 3
	}}
	sem := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(1, "Cat Tee", 0.85),    // sem rank 1 — appears in both, should win
		mkHit(3, "Pet Hoodie", 0.75), // sem rank 2
		mkHit(4, "Dog Bandana", 0.7), // sem rank 3 — NOT in lex
	}}
	r := New(lex, sem)
	hits, trace, err := r.TopKWithTrace(context.Background(), 7, "pet products", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("TopKWithTrace: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected fused hits")
	}
	if hits[0].Asset.ID != 1 {
		t.Errorf("asset in both rankings should rank #1; got id=%d", hits[0].Asset.ID)
	}
	// Verify trace exposes both ranks for the top hit.
	if len(trace.Selected) == 0 {
		t.Fatal("trace.Selected empty")
	}
	top := trace.Selected[0]
	if top.BM25Rank != 1 || top.SemanticRank != 1 {
		t.Errorf("top hit ranks: bm25=%d sem=%d; want 1,1", top.BM25Rank, top.SemanticRank)
	}
	if top.RRFScore <= 0 {
		t.Error("RRFScore should be positive for fused hit")
	}
}

// Pinned-asset survival (goal directive §4): an asset pinned at the
// lexical layer surfaces in lex's top results regardless of semantic
// weakness. RRF inherits this — pinned asset gets a lex rank, gets
// a fusion score floor.
func TestRRF_PinnedAssetSurvivesWeakSemantic(t *testing.T) {
	// Hybrid promotes pinned asset to rank 1 even with weak text match.
	lex := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(1, "Pinned CTA", 0.30), // weak score but rank 1 in lex
		mkHit(2, "Other", 0.25),
	}}
	// Semantic ranks the pinned asset LAST (irrelevant to query).
	sem := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(2, "Other", 0.85),
		mkHit(3, "Third", 0.80),
		mkHit(1, "Pinned CTA", 0.30), // sem rank 3
	}}
	r := New(lex, sem)
	hits, _, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)

	// Pinned asset should rank competitively because lex rank 1 dominates.
	// RRF for id=1: 1/(60+1) + 1/(60+3) = 0.01639 + 0.01587 = 0.03226
	// RRF for id=2: 1/(60+2) + 1/(60+1) = 0.01613 + 0.01639 = 0.03252
	// Very close. Pinned doesn't necessarily rank #1, but it must
	// SURVIVE (be in top-k).
	found := false
	for _, h := range hits {
		if h.Asset.ID == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Error("pinned asset eliminated by RRF; goal directive §4 violation")
	}
}

// Both upstreams fail → error surfaced, no hits.
func TestRRF_BothFail_ErrorPropagated(t *testing.T) {
	lex := &scriptedSearcher{err: errors.New("lex down")}
	sem := &scriptedSearcher{err: errors.New("sem down")}
	r := New(lex, sem)
	_, _, err := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if err == nil {
		t.Fatal("expected error when both upstreams fail")
	}
}

// One upstream fails → graceful degradation to the other.
func TestRRF_SingleUpstreamFail_GracefulDegrade(t *testing.T) {
	lex := &scriptedSearcher{hits: []retrieval.Hit{mkHit(1, "Cat Tee", 0.9)}}
	sem := &scriptedSearcher{err: errors.New("sem down")}
	r := New(lex, sem)
	hits, _, err := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("single-upstream failure should not error; got %v", err)
	}
	if len(hits) != 1 {
		t.Errorf("lex-only ranking should produce 1 hit; got %d", len(hits))
	}
}

// Low-confidence behavior (§7): when ALL fused scores are below
// MinConfidence, no hits returned. Caller orchestrator asks for
// clarification rather than hallucinating.
func TestRRF_LowConfidence_ReturnsEmpty(t *testing.T) {
	// Single asset, only in semantic, ranked very low (#100). Score is tiny.
	semHits := make([]retrieval.Hit, 100)
	for i := range semHits {
		semHits[i] = mkHit(int64(i+1), "x", 0.5)
	}
	sem := &scriptedSearcher{hits: semHits}
	r := New(&scriptedSearcher{hits: nil}, sem)
	r.MinConfidence = 0.02 // very high threshold to force trigger
	hits, trace, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if len(hits) != 0 {
		t.Errorf("low confidence should produce empty; got %d hits", len(hits))
	}
	if trace.TotalByReason["low_confidence"] == 0 {
		t.Error("low_confidence reason should be recorded in trace")
	}
}

// Trace explainability: every selected hit carries both per-searcher
// ranks and the RRF score. Replay UI relies on this.
func TestRRF_TraceCarriesBothRanks(t *testing.T) {
	lex := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(1, "Both", 0.9),
	}}
	sem := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(1, "Both", 0.85),
	}}
	r := New(lex, sem)
	_, trace, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if len(trace.Selected) == 0 {
		t.Fatal("empty trace")
	}
	if trace.Selected[0].BM25Rank == 0 {
		t.Error("bm25_rank missing")
	}
	if trace.Selected[0].SemanticRank == 0 {
		t.Error("semantic_rank missing")
	}
	if trace.Selected[0].RRFScore == 0 {
		t.Error("rrf_score missing")
	}
	if trace.Selected[0].Reason == "" {
		t.Error("reason should describe fusion")
	}
}

// SQLite path (lex only, semantic=nil): RRF effectively just sorts by
// lex rank. Output should match lex order.
func TestRRF_LexOnly_PreservesLexOrder(t *testing.T) {
	lex := &scriptedSearcher{hits: []retrieval.Hit{
		mkHit(1, "A", 0.9),
		mkHit(2, "B", 0.8),
		mkHit(3, "C", 0.7),
	}}
	r := New(lex, nil)
	hits, _, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	for i, h := range hits {
		expected := int64(i + 1)
		if h.Asset.ID != expected {
			t.Errorf("hit[%d].ID = %d; want %d", i, h.Asset.ID, expected)
		}
	}
}

// k cap is honored.
func TestRRF_KCap(t *testing.T) {
	lex := &scriptedSearcher{hits: []retrieval.Hit{}}
	for i := int64(1); i <= 20; i++ {
		lex.hits = append(lex.hits, mkHit(i, "x", 0.5))
	}
	sem := &scriptedSearcher{hits: lex.hits}
	r := New(lex, sem)
	hits, trace, _ := r.TopKWithTrace(context.Background(), 7, "q", retrieval.SearchFilter{}, 5)
	if len(hits) != 5 {
		t.Errorf("k=5 must produce 5 hits; got %d", len(hits))
	}
	if trace.TotalByReason[retrieval.RejectTopKCap] == 0 {
		t.Error("overflow should be recorded as topk_cap rejections")
	}
}
