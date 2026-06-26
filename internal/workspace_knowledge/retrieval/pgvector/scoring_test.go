package pgvector

import (
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

func newTrace() retrieval.Trace {
	return retrieval.Trace{TotalByReason: map[retrieval.RejectionReason]int{}}
}

// scoreVectorHits converts distance→similarity, drops sub-threshold
// hits (recording the reason), and builds the semantic-primary
// breakdown for survivors.
func TestScoreVectorHits_ThresholdAndBreakdown(t *testing.T) {
	hits := []retrieval.VectorHit{
		mkVectorHit(1, "strong", 0.1, false, 0), // sim 0.9 — kept
		mkVectorHit(2, "weak", 0.8, false, 0),   // sim 0.2 — dropped (< 0.60)
	}
	tr := newTrace()
	scored := scoreVectorHits(hits, DefaultMinSimilarity, &tr)

	if len(scored) != 1 || scored[0].hit.AssetID != 1 {
		t.Fatalf("kept = %+v, want only asset 1", scored)
	}
	if tr.TotalByReason[retrieval.RejectSemanticThreshold] != 1 {
		t.Errorf("threshold rejections = %d, want 1", tr.TotalByReason[retrieval.RejectSemanticThreshold])
	}
	// similarity = 1 - 0.1 = 0.9; Semantic = 0.70 * 0.9.
	if scored[0].similarity != 0.9 {
		t.Errorf("similarity = %v, want 0.9", scored[0].similarity)
	}
	if got, want := scored[0].breakdown.Semantic, 0.70*0.9; got != want {
		t.Errorf("Semantic breakdown = %v, want %v", got, want)
	}
}

// Negative distances (similarity > 1) clamp to 1, not above.
func TestScoreVectorHits_ClampUpper(t *testing.T) {
	tr := newTrace()
	scored := scoreVectorHits([]retrieval.VectorHit{mkVectorHit(1, "x", -0.5, false, 0)}, DefaultMinSimilarity, &tr)
	if len(scored) != 1 || scored[0].similarity != 1.0 {
		t.Fatalf("similarity = %+v, want clamped to 1.0", scored)
	}
}

// A pinned asset gets the 0.20 pin contribution in its breakdown.
func TestScoreVectorHits_PinContribution(t *testing.T) {
	tr := newTrace()
	scored := scoreVectorHits([]retrieval.VectorHit{mkVectorHit(1, "x", 0.1, true, 0)}, DefaultMinSimilarity, &tr)
	if len(scored) != 1 || scored[0].breakdown.Pin != 0.20 {
		t.Fatalf("pin breakdown = %+v, want 0.20", scored)
	}
}

// buildVectorOutput caps to k and records the overflow as RejectTopKCap.
func TestBuildVectorOutput_Cap(t *testing.T) {
	scored := []scoredVectorHit{
		{hit: mkVectorHit(1, "a", 0.1, false, 0), similarity: 0.9},
		{hit: mkVectorHit(2, "b", 0.2, false, 0), similarity: 0.8},
	}
	tr := newTrace()
	out := buildVectorOutput(scored, 1, &tr)

	if len(out) != 1 || out[0].Asset.ID != 1 {
		t.Fatalf("kept = %+v, want single asset 1", out)
	}
	if tr.TotalByReason[retrieval.RejectTopKCap] != 1 {
		t.Errorf("topk-cap rejections = %d, want 1", tr.TotalByReason[retrieval.RejectTopKCap])
	}
}
