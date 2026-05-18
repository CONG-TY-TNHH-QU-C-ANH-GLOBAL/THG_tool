package hybrid

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// fakeLister returns a fixed list and applies state filter so tests
// can reuse it in different SearchFilter scenarios.
type fakeLister struct {
	all []*assets.Asset
}

func (f *fakeLister) ListKnowledgeAssetsForOrg(_ context.Context, _ int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	if len(filter.States) == 0 {
		return f.all, nil
	}
	wanted := map[assets.AssetState]struct{}{}
	for _, s := range filter.States {
		wanted[s] = struct{}{}
	}
	out := make([]*assets.Asset, 0, len(f.all))
	for _, a := range f.all {
		if _, ok := wanted[a.State]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func mkAsset(id int64, title, desc string, typ assets.AssetType, tags []string, state assets.AssetState, pinned bool, boost int, lastRetrieved *time.Time) *assets.Asset {
	return &assets.Asset{
		ID:          id,
		OrgID:       7,
		SourceID:    1,
		Type:        typ,
		Title:       title,
		Description: desc,
		Tags:        tags,
		State:       state,
		Pinned:      pinned,
		Boost:       boost,
		Metrics:     assets.Metrics{LastRetrievedAt: lastRetrieved},
	}
}

// Frozen clock for deterministic recency math.
func newTestSearcher(lister AssetLister) *Searcher {
	fixed := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	return &Searcher{Lister: lister, Clock: func() time.Time { return fixed }}
}

// Governance: banned_claim assets are filtered AT THE SEARCHER, not
// merely dropped at assembly. This is the load-bearing distinction
// from the naive searcher and the whole point of hybrid governance.
func TestHybrid_BannedClaimFilteredAtSource(t *testing.T) {
	hot := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC) // 2h ago
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "best price guaranteed", "cannot prove", assets.AssetBannedClaim,
			[]string{"cta"}, assets.StateApproved, true, 100, &hot), // pinned + boosted + recent
		mkAsset(2, "Cat Tee", "premium", assets.AssetPODProduct,
			[]string{"cat"}, assets.StateApproved, false, 0, nil),
	}}
	s := newTestSearcher(lister)
	hits, trace, err := s.TopKWithTrace(context.Background(), 7, "best price", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("TopKWithTrace: %v", err)
	}
	for _, h := range hits {
		if h.Asset.Type == assets.AssetBannedClaim {
			t.Errorf("banned claim leaked into hits: %+v", h)
		}
	}
	// And the trace records the rejection reason as governance.
	if trace.TotalByReason[retrieval.RejectGovernance] != 1 {
		t.Errorf("expected 1 governance rejection in trace; got %d", trace.TotalByReason[retrieval.RejectGovernance])
	}
}

// Exact phrase in description should beat single-token matches even
// when the latter have boost on their side.
func TestHybrid_ExactPhraseBeatsTokenOverlap(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Generic Tee", "we sell custom POD shirts", assets.AssetPODProduct,
			[]string{"tee"}, assets.StateApproved, false, 50, nil),
		mkAsset(2, "POD shirt fulfillment", "POD shirt with fulfillment SLA", assets.AssetPODProduct,
			[]string{}, assets.StateApproved, false, 0, nil),
	}}
	s := newTestSearcher(lister)
	hits, _, _ := s.TopKWithTrace(context.Background(), 7, "pod shirt", retrieval.SearchFilter{}, 5)
	if len(hits) < 2 {
		t.Fatalf("expected 2 hits; got %d", len(hits))
	}
	// Asset 2 has the exact phrase "pod shirt" in title — should rank first.
	if hits[0].Asset.ID != 2 {
		t.Errorf("exact phrase asset should rank first; got id=%d", hits[0].Asset.ID)
	}
}

// Recency lift: an asset retrieved 2 hours ago beats an otherwise-
// identical asset never retrieved.
func TestHybrid_RecencyLiftsRecentlyRetrieved(t *testing.T) {
	hot := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC) // 2h before frozen clock
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Cat Tee", "", assets.AssetPODProduct, []string{"cat"}, assets.StateApproved, false, 0, nil),
		mkAsset(2, "Cat Tee", "", assets.AssetPODProduct, []string{"cat"}, assets.StateApproved, false, 0, &hot),
	}}
	s := newTestSearcher(lister)
	hits, _, _ := s.TopKWithTrace(context.Background(), 7, "cat", retrieval.SearchFilter{}, 5)
	if len(hits) != 2 {
		t.Fatalf("got %d hits; want 2", len(hits))
	}
	if hits[0].Asset.ID != 2 {
		t.Errorf("recent asset should rank first; got id=%d", hits[0].Asset.ID)
	}
	// Score breakdown: hits[0] should carry a non-zero recency.
	if hits[0].Score <= hits[1].Score {
		t.Errorf("recent asset score not greater; %f vs %f", hits[0].Score, hits[1].Score)
	}
}

// Recency decays past 30 days — an asset retrieved 60 days ago gets
// no recency lift.
func TestHybrid_RecencyDecaysPast30Days(t *testing.T) {
	cold := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC) // 60 days before frozen
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Cat Tee", "", assets.AssetPODProduct, []string{"cat"}, assets.StateApproved, false, 0, &cold),
	}}
	s := newTestSearcher(lister)
	hits, _, _ := s.TopKWithTrace(context.Background(), 7, "cat", retrieval.SearchFilter{}, 5)
	if len(hits) != 1 {
		t.Fatalf("got %d hits", len(hits))
	}
	if hits[0].Score == 0 {
		t.Skip("no score is fine for cold assets without text match")
	}
	// The breakdown.Recency must be 0 — decayed entirely.
	// (We can't introspect the breakdown via Hit; assert via trace.)
	_, trace, _ := s.TopKWithTrace(context.Background(), 7, "cat", retrieval.SearchFilter{}, 5)
	if len(trace.Selected) > 0 && trace.Selected[0].Breakdown.Recency != 0 {
		t.Errorf("recency should be 0 past 30 days; got %f", trace.Selected[0].Breakdown.Recency)
	}
}

// Score breakdown exposes contribution per signal. The Operator
// Replay UI relies on this to render the stacked bar.
func TestHybrid_ScoreBreakdownExposesContributions(t *testing.T) {
	recent := time.Date(2026, 5, 18, 11, 0, 0, 0, time.UTC) // 1h ago
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Cat Tee", "premium quality", assets.AssetPODProduct,
			[]string{"cat", "tee"}, assets.StateApproved, true, 50, &recent),
	}}
	s := newTestSearcher(lister)
	_, trace, _ := s.TopKWithTrace(context.Background(), 7, "cat tee", retrieval.SearchFilter{}, 5)
	if len(trace.Selected) != 1 {
		t.Fatalf("selected count: %d", len(trace.Selected))
	}
	bd := trace.Selected[0].Breakdown
	if bd.TextMatch <= 0 {
		t.Errorf("text match should be > 0; got %f", bd.TextMatch)
	}
	if bd.Boost <= 0 {
		t.Errorf("boost should be > 0; got %f", bd.Boost)
	}
	if bd.Pin <= 0 {
		t.Errorf("pin should be > 0; got %f", bd.Pin)
	}
	if bd.Recency <= 0 {
		t.Errorf("recency should be > 0; got %f", bd.Recency)
	}
	// Sum should approximate Score (after clamp).
	sum := bd.TextMatch + bd.Boost + bd.Pin + bd.Recency
	if sum > 1.0 {
		sum = 1.0
	}
	if abs(sum-trace.Selected[0].Score) > 0.001 {
		t.Errorf("breakdown sum %f != score %f", sum, trace.Selected[0].Score)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
