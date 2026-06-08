package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/products"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// fakeSearcher returns canned hits so CandidatesForLead's filtering can be tested
// without a store.
type fakeSearcher struct{ hits []retrieval.Hit }

func (f fakeSearcher) TopK(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, error) {
	return f.hits, nil
}
func (f fakeSearcher) TopKWithTrace(ctx context.Context, orgID int64, query string, filter retrieval.SearchFilter, k int) ([]retrieval.Hit, retrieval.Trace, error) {
	return f.hits, retrieval.Trace{}, nil
}

func TestCandidatesForLead_SkipsBannedAndNonApproved_NoFakeRetrievalID(t *testing.T) {
	hits := []retrieval.Hit{
		{Asset: &assets.Asset{ID: 1, Type: assets.AssetSalesPlaybook, State: assets.StateApproved, Title: "ok"}, Score: 0.8},
		{Asset: &assets.Asset{ID: 2, Type: assets.AssetBannedClaim, State: assets.StateApproved, Title: "banned"}, Score: 0.7},
		{Asset: &assets.Asset{ID: 3, Type: assets.AssetFAQ, State: assets.StateHidden, Title: "hidden"}, Score: 0.6},
		{Asset: &assets.Asset{ID: 4, Type: assets.AssetFAQ, State: assets.StatePending, Title: "pending"}, Score: 0.5},
	}
	b := &Builder{Searcher: fakeSearcher{hits: hits}, K: 6}
	out, retrievalID, err := b.CandidatesForLead(context.Background(), 1, "lead")
	if err != nil {
		t.Fatalf("CandidatesForLead: %v", err)
	}
	if len(out) != 1 || out[0].AssetID != 1 {
		t.Fatalf("only the approved non-banned asset should survive, got %+v", out)
	}
	if retrievalID != "" {
		t.Fatalf("must not mint a fake retrieval_id, got %q", retrievalID)
	}
}

func TestCandidateFromHit_ProductSurfacesImageAndPrice(t *testing.T) {
	lo, hi := 4.5, 9.0
	pv := products.PayloadV1{
		SchemaVersion: 1,
		DisplaySKU:    "ABC",
		Name:          "Áo thun",
		PriceMin:      &lo,
		PriceMax:      &hi,
		Currency:      "USD",
		Images:        []string{"http://img/a.jpg", "http://img/b.jpg"},
	}
	payload, err := json.Marshal(pv)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	h := retrieval.Hit{
		Asset: &assets.Asset{ID: 5, Type: assets.AssetPODProduct, Title: "Áo thun", Payload: payload},
		Score: 0.91,
	}
	c := candidateFromHit(h)
	if c.AssetID != 5 || c.SKU != "ABC" {
		t.Fatalf("asset_id/sku not surfaced: %+v", c)
	}
	if c.ImageURL != "http://img/a.jpg" { // Images[0] is the primary
		t.Fatalf("primary image not surfaced, got %q", c.ImageURL)
	}
	if c.PriceText != "4.5-9 USD" {
		t.Fatalf("price not formatted, got %q", c.PriceText)
	}
	if c.Score != 0.91 {
		t.Fatalf("score not carried, got %v", c.Score)
	}
}

func TestCandidateFromHit_SinglePriceNoRange(t *testing.T) {
	p := 22.0
	pv := products.PayloadV1{SchemaVersion: 1, DisplaySKU: "X", PriceMin: &p, PriceMax: &p, Currency: "USD"}
	payload, _ := json.Marshal(pv)
	c := candidateFromHit(retrieval.Hit{Asset: &assets.Asset{ID: 1, Type: assets.AssetPODProduct, Payload: payload}})
	if c.PriceText != "22 USD" {
		t.Fatalf("equal min/max should render single price, got %q", c.PriceText)
	}
}

func TestCandidateFromHit_NonProductNoSKU(t *testing.T) {
	c := candidateFromHit(retrieval.Hit{
		Asset: &assets.Asset{ID: 8, Type: assets.AssetSalesPlaybook, Title: "US fulfillment", Description: "We fulfill from VN/CN to US warehouses."},
		Score: 0.5,
	})
	if c.SKU != "" || c.PriceText != "" {
		t.Fatalf("non-product should have no sku/price, got %+v", c)
	}
	if c.Kind != string(assets.AssetSalesPlaybook) || c.Title != "US fulfillment" {
		t.Fatalf("kind/title not mapped, got %+v", c)
	}
}
