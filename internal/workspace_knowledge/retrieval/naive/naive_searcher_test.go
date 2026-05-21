package naive

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// fakeLister returns a fixed list, asserting on the filter so tests
// can verify that EffectiveStates is honored.
type fakeLister struct {
	all          []*assets.Asset
	gotFilter    assets.ListFilter
	gotOrgID     int64
}

func (f *fakeLister) ListAssetsForOrg(_ context.Context, orgID int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	f.gotFilter = filter
	f.gotOrgID = orgID
	// Apply state filter so tests can use fakeLister like the real one
	// — otherwise scoring logic looks fine but doesn't prove the
	// filter is being respected.
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

func mkAsset(id int64, title string, tags []string, state assets.AssetState, pinned bool, boost int) *assets.Asset {
	return &assets.Asset{
		ID:       id,
		OrgID:    7,
		SourceID: 1,
		Type:     assets.AssetPODProduct,
		Title:    title,
		Tags:     tags,
		State:    state,
		Pinned:   pinned,
		Boost:    boost,
	}
}

func TestTopK_TitleAndTagOverlap(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Custom Cat Tee", []string{"cat", "tee"}, assets.StateApproved, false, 0),
		mkAsset(2, "Dog Mug", []string{"dog", "mug"}, assets.StateApproved, false, 0),
		mkAsset(3, "Cat Mug Ceramic", []string{"cat", "mug"}, assets.StateApproved, false, 0),
	}}
	s := New(lister)
	hits, err := s.TopK(context.Background(), 7, "cat tee POD", retrieval.SearchFilter{}, 5)
	if err != nil {
		t.Fatalf("TopK: %v", err)
	}
	// Asset 1 (cat tee) should be highest — overlap on both tokens.
	// Asset 3 (cat mug) second — overlap on cat.
	// Asset 2 (dog mug) zero overlap → excluded.
	if len(hits) != 2 {
		t.Fatalf("got %d hits; want 2", len(hits))
	}
	if hits[0].Asset.ID != 1 {
		t.Errorf("top hit should be cat tee (id=1); got id=%d", hits[0].Asset.ID)
	}
	if hits[1].Asset.ID != 3 {
		t.Errorf("second hit should be cat mug (id=3); got id=%d", hits[1].Asset.ID)
	}
	if hits[0].Score <= hits[1].Score {
		t.Errorf("scores not monotonic: %f vs %f", hits[0].Score, hits[1].Score)
	}
}

func TestTopK_PinnedAlwaysSurfaces(t *testing.T) {
	// Pinned asset that has ZERO query overlap must still be returned.
	// Documented invariant: pin is ~25% of total score, so even a
	// completely irrelevant pinned asset clears the >0 cutoff.
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Custom Cat Tee", []string{"cat", "tee"}, assets.StateApproved, false, 0),
		mkAsset(2, "Soft CTA — DM Invite", []string{"cta"}, assets.StateApproved, true, 0),
	}}
	s := New(lister)
	hits, _ := s.TopK(context.Background(), 7, "cat tee", retrieval.SearchFilter{}, 5)
	foundCTA := false
	for _, h := range hits {
		if h.Asset.ID == 2 {
			foundCTA = true
			if h.Reason != "pinned" {
				t.Errorf("pinned-only reason should be 'pinned'; got %q", h.Reason)
			}
		}
	}
	if !foundCTA {
		t.Error("pinned asset with zero text overlap must still appear in TopK")
	}
}

func TestTopK_BoostInfluencesOrder(t *testing.T) {
	// Two assets with identical text overlap; boost breaks the tie.
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Cat Tee A", []string{"cat", "tee"}, assets.StateApproved, false, 0),
		mkAsset(2, "Cat Tee B", []string{"cat", "tee"}, assets.StateApproved, false, 80),
	}}
	s := New(lister)
	hits, _ := s.TopK(context.Background(), 7, "cat tee", retrieval.SearchFilter{}, 5)
	if len(hits) != 2 {
		t.Fatalf("got %d hits; want 2", len(hits))
	}
	if hits[0].Asset.ID != 2 {
		t.Errorf("boosted asset should rank first; got id=%d", hits[0].Asset.ID)
	}
}

func TestTopK_ApprovedOnlyByDefault(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Cat Tee — approved", []string{"cat"}, assets.StateApproved, false, 0),
		mkAsset(2, "Cat Tee — hidden", []string{"cat"}, assets.StateHidden, false, 0),
		mkAsset(3, "Cat Tee — pending", []string{"cat"}, assets.StatePending, false, 0),
	}}
	s := New(lister)
	hits, _ := s.TopK(context.Background(), 7, "cat", retrieval.SearchFilter{}, 5)
	// Only the approved asset should surface.
	if len(hits) != 1 {
		t.Fatalf("got %d hits; want 1 (approved only)", len(hits))
	}
	if hits[0].Asset.ID != 1 {
		t.Errorf("approved-only filter leaked; got id=%d", hits[0].Asset.ID)
	}
	// And the lister was called with a States filter, not empty.
	if len(lister.gotFilter.States) != 1 || lister.gotFilter.States[0] != assets.StateApproved {
		t.Errorf("EffectiveStates default not propagated to lister; got %v", lister.gotFilter.States)
	}
}

func TestTopK_TagFilterAndOf(t *testing.T) {
	lister := &fakeLister{all: []*assets.Asset{
		mkAsset(1, "Cat Tee", []string{"cat", "pet"}, assets.StateApproved, false, 0),
		mkAsset(2, "Cat Hoodie", []string{"cat", "wholesale"}, assets.StateApproved, false, 0),
	}}
	s := New(lister)
	hits, _ := s.TopK(context.Background(), 7, "cat", retrieval.SearchFilter{
		Tags: []string{"wholesale"},
	}, 5)
	// Only asset 2 has the wholesale tag.
	if len(hits) != 1 || hits[0].Asset.ID != 2 {
		t.Errorf("tag filter should leave only id=2; got %+v", hits)
	}
}

func TestTopK_KIsRespected(t *testing.T) {
	all := make([]*assets.Asset, 0, 10)
	for i := int64(1); i <= 10; i++ {
		all = append(all, mkAsset(i, "Cat product", []string{"cat"}, assets.StateApproved, false, 0))
	}
	s := New(&fakeLister{all: all})
	hits, _ := s.TopK(context.Background(), 7, "cat", retrieval.SearchFilter{}, 3)
	if len(hits) != 3 {
		t.Errorf("k=3 must produce ≤3 hits; got %d", len(hits))
	}
}
