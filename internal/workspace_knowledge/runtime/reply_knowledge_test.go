package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// Deliberately ignores filters to test the consumer's defensive checks.
type replyTestSearcher struct{ hits []retrieval.Hit }

type productBudgetStore struct{ multilingualStore }

func (s *productBudgetStore) ListAssetsForOrg(ctx context.Context, org int64, filter assets.ListFilter) ([]*assets.Asset, error) {
	items, err := s.multilingualStore.ListAssetsForOrg(ctx, org, filter)
	var selected []*assets.Asset
	for _, item := range items {
		for _, kind := range filter.Types {
			if item.Type == kind {
				selected = append(selected, item)
			}
		}
	}
	return selected, err
}

type boundedReplySearcher struct {
	replyTestSearcher
	requested int
}

func (s *boundedReplySearcher) TopKWithTrace(_ context.Context, _ int64, _ string, _ retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	s.requested = k
	return s.hits[:min(k, len(s.hits))], retrieval.Trace{}, nil
}

func TestReplyKnowledgeLooksPastLegacyRowsAndCapsContext(t *testing.T) {
	searcher := &boundedReplySearcher{}
	for i := 0; i < 20; i++ {
		item := &assets.Asset{OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved,
			Description: "LEGACY PRIVATE DESCRIPTION", Payload: []byte(`{}`)}
		if i >= 4 {
			item.Payload, _ = json.Marshal(assets.ReplyKnowledgeV1{Schema: "company_reply_v1", Audience: "public_reference",
				Verification: "public_source_checked", Facts: []string{"public-fact-" + strconv.Itoa(i)}, SourceIDs: []string{"web-express"}})
		}
		searcher.hits = append(searcher.hits, retrieval.Hit{Asset: item})
	}
	b := &Builder{Searcher: searcher}
	got, err := b.ReplyKnowledgeForLead(context.Background(), 7, "shipping")
	var facts []assets.ReplyKnowledgeV1
	if err != nil || json.Unmarshal([]byte(got), &facts) != nil || len(facts) != 4 {
		t.Fatalf("missing bounded public context: %q, %v", got, err)
	}
	if searcher.requested != 12 || facts[0].Facts[0] != "public-fact-4" || strings.Contains(got, "PRIVATE") {
		t.Fatalf("legacy rows starved or leaked into context: %q", got)
	}
}

func (s replyTestSearcher) TopK(context.Context, int64, string, retrieval.SearchFilter, int) ([]retrieval.Hit, error) {
	return s.hits, nil
}
func (s replyTestSearcher) TopKWithTrace(context.Context, int64, string, retrieval.SearchFilter, int) ([]retrieval.Hit, retrieval.Trace, error) {
	return s.hits, retrieval.Trace{}, nil
}

func TestReplyKnowledgeRejectsUnsafeAssets(t *testing.T) {
	payload, _ := json.Marshal(assets.ReplyKnowledgeV1{Schema: "company_reply_v1", Audience: "public_reference",
		Verification: "public_source_checked", Facts: []string{"Public warehouse capability"}, SourceIDs: []string{"web-warehouse"}})
	valid := assets.Asset{OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved, Payload: payload,
		Description: "PRIVATE MARGIN", Title: "RAW INTERNAL TITLE"}
	var hits []retrieval.Hit
	for _, variant := range []string{"valid", "pending", "hidden", "tenant", "untyped", "internal", "legacy", "banned", "product"} {
		item := valid
		switch variant {
		case "pending":
			item.State = assets.StatePending
		case "hidden":
			item.State = assets.StateHidden
		case "tenant":
			item.OrgID = 8
		case "untyped":
			item.Payload = []byte(`{"facts":["PRIVATE"]}`)
		case "internal":
			item.Payload = []byte(strings.Replace(string(payload), "public_reference", "sales_internal", 1))
		case "legacy":
			item.Payload = []byte(strings.Replace(string(payload), "public_source_checked", "legacy_source_checked", 1))
		case "banned":
			item.Type = assets.AssetBannedClaim
		case "product":
			item.Type = assets.AssetPODProduct
		}
		hits = append(hits, retrieval.Hit{Asset: &item})
	}
	builder := &Builder{Searcher: replyTestSearcher{hits}}
	got, err := builder.ReplyKnowledgeForLead(context.Background(), 7, "warehouse")
	if err != nil || strings.Count(got, "Public warehouse capability") != 1 || strings.Contains(got, "PRIVATE") || strings.Contains(got, "RAW INTERNAL") {
		t.Fatalf("unsafe context: %s, %v", got, err)
	}
}

func TestProductsHaveSeparateRetrievalBudget(t *testing.T) {
	store := &productBudgetStore{}
	for i := 1; i <= 10; i++ {
		store.items = append(store.items, &assets.Asset{ID: int64(i), OrgID: 7, Type: assets.AssetFAQ, State: assets.StateApproved, Title: "custom mug printing catalog"})
	}
	store.items = append(store.items, &assets.Asset{ID: 11, OrgID: 7, Type: assets.AssetPODProduct, State: assets.StateApproved, Title: "custom mug"})
	b := NewBuilder(store)
	hits, _, err := b.ProductCandidatesForLead(context.Background(), 7, "custom mug")
	if err != nil || len(store.last.Types) != 2 || !store.last.ExcludeUnhealthySources || len(hits) != 1 || hits[0].AssetID != 11 {
		t.Fatalf("product was crowded out or filter lost: %+v, %+v, %v", hits, store.last, err)
	}
}
