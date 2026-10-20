package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval/hybrid"
)

type multilingualStore struct {
	items []*assets.Asset
	last  assets.ListFilter
}

func (s *multilingualStore) ListAssetsForOrg(_ context.Context, org int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	s.last = filter
	var out []*assets.Asset
	for _, item := range s.items {
		if item.OrgID == org && item.State == assets.StateApproved {
			out = append(out, item)
		}
	}
	return out, nil
}

func (*multilingualStore) HasPGVector(context.Context) bool { return false }
func (*multilingualStore) QueryNearestVectors(context.Context, int64, []float32, string, retrieval.VectorFilter, int) ([]retrieval.VectorHit, error) {
	return nil, fmt.Errorf("vector query must not run without capability")
}

func TestBuildersUseMultilingualLexicalRetrieval(t *testing.T) {
	store := &multilingualStore{items: []*assets.Asset{
		{ID: 1, OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved, Title: "Dịch vụ POD"},
		{ID: 2, OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved, Title: "Kho Mỹ"},
		{ID: 3, OrgID: 8, Type: assets.AssetFAQ, State: assets.StateApproved, Title: "dịch vụ kho Mỹ"},
		{ID: 4, OrgID: 7, Type: assets.AssetFAQ, State: assets.StatePending, Title: "dịch vụ kho Mỹ"},
		{ID: 5, OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved, Title: "Dịch vụ vận chuyển"},
	}}
	for _, builder := range []*Builder{NewBuilder(store), NewBuilderWithVector(context.Background(), store, nil)} {
		hits, trace, err := builder.Searcher.TopKWithTrace(context.Background(), 7, "dich vu kho My", retrieval.SearchFilter{}, 3)
		if err != nil || len(hits) == 0 || hits[0].Asset.ID != 2 || trace.SearcherImpl != hybrid.MultilingualSearcherImpl {
			t.Fatalf("builder did not select the v2 warehouse result: %+v, %+v, %v", hits, trace, err)
		}
		for _, hit := range hits {
			if hit.Asset.OrgID != 7 || hit.Asset.State != assets.StateApproved {
				t.Fatalf("filter contract violated: %+v", hit.Asset)
			}
		}
		if !store.last.ExcludeUnhealthySources || len(store.last.States) != 1 || store.last.States[0] != assets.StateApproved {
			t.Fatalf("runtime lost source health/state filtering: %+v", store.last)
		}
	}
}
