package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestPODSelectionSkipsIndexAndMarketplaceLinks(t *testing.T) {
	const good = "https://thgfulfill.com/vi/catalog?productId=cup-1"
	for _, bad := range []string{
		"https://thgfulfill.com", "https://thgfulfill.com/vi/", "https://thgfulfill.com/vi/catalog",
		"https://thgfulfill.com/vi/catalog?search=cup", "https://thgfulfill.com/products/",
		"https://thgfulfill.com/vi/catalog?productId=undefined", "https://detail.1688.com/offer/123.html",
		"https://item.taobao.com/item.htm?id=123", "https://m.tb.cn/short-link",
	} {
		got := PickSuggestedProductDetails([]models.KnowledgeCandidate{
			{Kind: "POD_product", Title: "Cốc sứ", SourceURL: bad},
			{Kind: "POD_product", Title: "Cốc sứ", SourceURL: good},
		})
		if got.URL != good {
			t.Errorf("selected non-catalog/product URL %q instead of the next valid item", got.URL)
		}
	}
	got := PickSuggestedProductDetails([]models.KnowledgeCandidate{{Kind: "POD_product", SourceURL: good}})
	if got.URL != "" {
		t.Fatal("an unnamed item must not be offered")
	}
}

func TestSupplierSelectionSkipsNonListingLinks(t *testing.T) {
	good := supplierCandidate()
	for _, link := range []string{
		"https://1688.com/", "https://www.taobao.com/", "https://s.1688.com/selloffer/offer_search.htm?keywords=cup",
		"https://shop.taobao.com/index.htm", "https://detail.1688.com/offer/undefined.html",
		"https://detail.1688.com/offer/123garbage.html", "https://item.taobao.com/item.htm?id=undefined",
	} {
		bad := supplierCandidate()
		bad.SourceURL = link
		got := PickSuggestedSupplier([]models.KnowledgeCandidate{bad, good})
		if got == nil || got.URL != good.SourceURL {
			t.Fatalf("non-listing supplier URL selected: %q, %+v", link, got)
		}
	}
}

func TestInvalidLiveSupplierURLCannotReachReplyOrShipping(t *testing.T) {
	for _, link := range []string{"https://1688.com/", "https://thgfulfill.com/vi/catalog?productId=123", "https://unrelated.example/item/123", " https://detail.1688.com/offer/123.html "} {
		lookup := func(context.Context, string) (*ResolvedSupplier, error) {
			weight := 0.4
			return &ResolvedSupplier{Match: &models.SupplierMatch{Name: "Áo hoodie", URL: link, PriceText: "¥20"}, WeightKG: &weight}, nil
		}
		quote := func(context.Context, models.ShippingRequest) (*models.ShippingReference, error) {
			t.Error("non-listing must not reach shipping")
			return nil, nil
		}
		got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 7, "Cần nhập 300 áo hoodie từ 1688 về Mỹ", "Lan", quote, lookup)
		if got.Supplier.HasOffer() || got.Reply == "" || got.SourcingNote == "" || strings.Contains(got.Reply, "https://") {
			t.Errorf("invalid live URL leaked or clarification lost: %+v", got)
		}
	}
}

func TestProductLinkPolicyKeepsSupportedListingShapes(t *testing.T) {
	for _, link := range []string{
		"https://detail.1688.com/offer/123.html?from=share", "https://m.1688.com/offer/123.html",
		"https://item.taobao.com/item.htm?id=123&skuId=456", "https://h5.m.taobao.com/awp/core/detail.htm?id=123",
		"https://detail.tmall.com/item.htm?id=123", "https://main.m.taobao.com/detail/detail.html?id=123",
	} {
		if !marketplaceProductURL(link) {
			t.Errorf("supported listing rejected: %s", link)
		}
	}
	for _, link := range []string{
		"https://thgfulfill.com/vi/catalog?productId=cup-1", "https://catalog.example/p/hoodie",
		"https://another-tenant.example/products/ceramic-mug",
	} {
		if catalogProductURL(link) != link {
			t.Errorf("tenant catalog item rejected: %s", link)
		}
	}
}

func TestLookupContinuesPastNonListingResults(t *testing.T) {
	const good = "https://detail.1688.com/offer/2.html"
	for _, tc := range []struct {
		name, firstURL string
		details        int
	}{
		{"search index skipped before detail", "https://s.1688.com/selloffer/offer_search.htm?keywords=hoodie", 1},
		{"invalid detail skipped", "https://detail.1688.com/offer/1.html", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price := 20.0
			client := &fakeSupplierReader{
				items: []suppliersourcing.SearchItem{
					{ID: "1", Title: "Áo hoodie", Link: tc.firstURL},
					{ID: "2", Title: "Áo hoodie", Link: good},
				},
				detailFn: func(id string) (*suppliersourcing.Product, error) {
					link := good
					if id == "1" {
						link = "https://1688.com/"
					}
					return &suppliersourcing.Product{Title: "Áo hoodie", Link: link, Price: &price}, nil
				},
			}
			got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập áo hoodie từ 1688")
			if err != nil || got == nil || got.Match.URL != good || client.details != tc.details {
				t.Fatalf("usable later offer lost: got=%+v details=%d err=%v", got, client.details, err)
			}
		})
	}
}
