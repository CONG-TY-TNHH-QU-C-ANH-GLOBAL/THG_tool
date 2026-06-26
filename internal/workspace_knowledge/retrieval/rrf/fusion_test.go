package rrf

import (
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

func newTrace() retrieval.Trace {
	return retrieval.Trace{TotalByReason: map[retrieval.RejectionReason]int{}}
}

// fuseRankings must assign 1-indexed ranks per searcher and union the
// two result sets keyed by asset ID.
func TestFuseRankings_UnionAndRanks(t *testing.T) {
	tr := newTrace()
	lex := []retrieval.Hit{mkHit(1, "a", 0.9), mkHit(2, "b", 0.8)}
	sem := []retrieval.Hit{mkHit(1, "a", 0.7), mkHit(3, "c", 0.6)}

	fused := fuseRankings(lex, sem, retrieval.Trace{}, &tr)

	if len(fused) != 3 {
		t.Fatalf("union size = %d, want 3", len(fused))
	}
	if fused[1].lexRank != 1 || fused[1].semRank != 1 {
		t.Errorf("asset 1 ranks = lex %d sem %d, want 1,1", fused[1].lexRank, fused[1].semRank)
	}
	if fused[2].lexRank != 2 || fused[2].semRank != 0 {
		t.Errorf("asset 2 (lex only) ranks = lex %d sem %d, want 2,0", fused[2].lexRank, fused[2].semRank)
	}
	if fused[3].semRank != 2 || fused[3].lexRank != 0 {
		t.Errorf("asset 3 (sem only) ranks = lex %d sem %d, want 0,2", fused[3].lexRank, fused[3].semRank)
	}
}

// The governance gate must drop banned_claim and hidden assets and
// record the reason, regardless of upstream filtering.
func TestFuseRankings_GovernanceGate(t *testing.T) {
	banned := mkHit(10, "banned", 0.9)
	banned.Asset.Type = assets.AssetBannedClaim
	hidden := mkHit(11, "hidden", 0.9)
	hidden.Asset.State = assets.StateHidden

	tr := newTrace()
	fused := fuseRankings([]retrieval.Hit{banned, hidden, mkHit(1, "ok", 0.5)}, nil, retrieval.Trace{}, &tr)

	if _, ok := fused[10]; ok {
		t.Error("banned_claim asset leaked into fusion")
	}
	if _, ok := fused[11]; ok {
		t.Error("hidden asset leaked into fusion")
	}
	if tr.TotalByReason[retrieval.RejectGovernance] != 1 {
		t.Errorf("governance rejections = %d, want 1", tr.TotalByReason[retrieval.RejectGovernance])
	}
	if tr.TotalByReason[retrieval.RejectStateFilter] != 1 {
		t.Errorf("state rejections = %d, want 1", tr.TotalByReason[retrieval.RejectStateFilter])
	}
}

// rankByRRF: an asset in both rankings outscores one in a single
// ranking, and results come back sorted descending.
func TestRankByRRF_DualBeatsSingleAndSorted(t *testing.T) {
	fused := map[int64]*fusedEntry{
		1: {asset: &assets.Asset{ID: 1}, lexRank: 1, semRank: 1}, // in both
		2: {asset: &assets.Asset{ID: 2}, lexRank: 2},            // lex only
	}
	scored := rankByRRF(fused, DefaultK)

	if len(scored) != 2 {
		t.Fatalf("scored len = %d, want 2", len(scored))
	}
	if scored[0].entry.asset.ID != 1 {
		t.Errorf("dual-ranking asset should sort first; got id=%d", scored[0].entry.asset.ID)
	}
	want := 1.0/float64(DefaultK+1) + 1.0/float64(DefaultK+1)
	if scored[0].rrfScore != want {
		t.Errorf("rrfScore = %v, want %v", scored[0].rrfScore, want)
	}
}

// selectAndBuild caps to k, records the overflow as RejectTopKCap, and
// composes the fusion reason string for the kept hits.
func TestSelectAndBuild_CapAndReason(t *testing.T) {
	scored := []scoredEntry{
		{entry: &fusedEntry{asset: &assets.Asset{ID: 1}, lexRank: 1, semRank: 2}, rrfScore: 0.5},
		{entry: &fusedEntry{asset: &assets.Asset{ID: 2}, lexRank: 3}, rrfScore: 0.2},
	}
	tr := newTrace()
	out := selectAndBuild(scored, 1, &tr)

	if len(out) != 1 || out[0].Asset.ID != 1 {
		t.Fatalf("kept = %+v, want single asset 1", out)
	}
	if tr.TotalByReason[retrieval.RejectTopKCap] != 1 {
		t.Errorf("topk-cap rejections = %d, want 1", tr.TotalByReason[retrieval.RejectTopKCap])
	}
	if out[0].Reason == "" {
		t.Error("expected a non-empty fusion reason string")
	}
}
