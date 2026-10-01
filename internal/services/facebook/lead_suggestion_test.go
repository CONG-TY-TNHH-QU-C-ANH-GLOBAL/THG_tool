package facebook

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

func TestPickSuggestedProduct_FirstProductWithLink(t *testing.T) {
	cands := []models.KnowledgeCandidate{
		{Kind: "sales_playbook", Title: "US fulfillment", SourceURL: "https://x/playbook"}, // not a product
		{Kind: "POD_product", Title: "Áo thun", SourceURL: ""},                             // product, no link → skip
		{Kind: "POD_product", Title: "Áo hoodie", SourceURL: "https://thgfulfill.com/vi/catalog?productId=p9"},
		{Kind: "POD_product", Title: "Áo khác", SourceURL: "https://thgfulfill.com/vi/catalog?productId=p10"},
	}
	name, url := PickSuggestedProduct(cands)
	if name != "Áo hoodie" || url != "https://thgfulfill.com/vi/catalog?productId=p9" {
		t.Fatalf("expected first product WITH a link, got name=%q url=%q", name, url)
	}
}

func TestPickSuggestedProduct_NoneWhenNoProductLink(t *testing.T) {
	cands := []models.KnowledgeCandidate{
		{Kind: "sales_playbook", Title: "playbook", SourceURL: "https://x/p"},
		{Kind: "POD_product", Title: "no-link", SourceURL: ""},
	}
	if name, url := PickSuggestedProduct(cands); name != "" || url != "" {
		t.Fatalf("expected empty when no product has a link, got name=%q url=%q", name, url)
	}
}

func TestPickSuggestedProductDetails_RequiresHTTPSAndAvailability(t *testing.T) {
	cands := []models.KnowledgeCandidate{
		{Kind: "POD_product", Title: "unsafe", SourceURL: "javascript:alert(1)"},
		{Kind: "POD_product", Title: "sold", SourceURL: "https://catalog.example/sold", Availability: "out_of_stock"},
		{Kind: "POD_product", Title: "ready", SourceURL: "https://catalog.example/p/1", ImageURL: "https://cdn.example/p/1.png", Availability: "in_stock"},
	}
	got := PickSuggestedProductDetails(cands)
	if got.Name != "ready" || got.URL != "https://catalog.example/p/1" || got.ImageURL != "https://cdn.example/p/1.png" {
		t.Fatalf("expected first safe available product, got %+v", got)
	}
}

func TestPickSuggestedProductDetails_DropsUnsafeImageOnly(t *testing.T) {
	got := PickSuggestedProductDetails([]models.KnowledgeCandidate{{
		Kind: "POD_product", Title: "ready", SourceURL: "https://catalog.example/p/1", ImageURL: "http://cdn.example/p/1.png",
	}})
	if got.URL == "" || got.ImageURL != "" {
		t.Fatalf("expected PDP but no insecure image, got %+v", got)
	}
}

func TestParseOrgAllowlist_FailClosed(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		allowed []int64
		denied  []int64
	}{
		{name: "empty", raw: "", denied: []int64{1}},
		{name: "ids", raw: "1, 9", allowed: []int64{1, 9}, denied: []int64{2, 0}},
		{name: "wildcard", raw: "*", allowed: []int64{1, 99}, denied: []int64{0, -1}},
		{name: "malformed invalidates all", raw: "1,nope", denied: []int64{1, 2}},
		{name: "wildcard cannot mix", raw: "*,1", denied: []int64{1, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := ParseOrgAllowlist(tt.raw)
			for _, id := range tt.allowed {
				if !list.Allows(id) {
					t.Fatalf("org %d should be allowed", id)
				}
			}
			for _, id := range tt.denied {
				if list.Allows(id) {
					t.Fatalf("org %d should be denied", id)
				}
			}
		})
	}
}

func TestMatchingCandidates_PODPriorityRequiresActualProductTerm(t *testing.T) {
	candidates := []models.KnowledgeCandidate{
		{Kind: "POD_product", Title: "Áo thun cotton", SourceURL: "https://thgfulfill.com/vi/catalog?productId=tee"},
		{Kind: "supplier_product", Title: "Viên bổ khớp cho chó", SourceURL: "https://detail.1688.com/offer/1.html"},
	}
	matched := matchingCandidates("Cần nhập viên bổ khớp cho chó từ 1688", candidates)
	if got := PickSuggestedProductDetails(matched); got.URL != "" {
		t.Fatalf("unrelated POD item must not be offered: %+v", got)
	}
	if len(matched) != 1 || matched[0].Kind != "supplier_product" {
		t.Fatalf("expected supplier-only match, got %+v", matched)
	}
	matched = matchingCandidates("Cần áo thun cotton in theo thiết kế", candidates)
	if got := PickSuggestedProductDetails(matched); got.URL == "" {
		t.Fatalf("matching POD item should win: %+v", matched)
	}
}

func TestMatchingCandidates_RejectsOneGenericOverlap(t *testing.T) {
	if matchesLeadProduct("Cần mua vải cotton may túi", "Áo thun cotton") {
		t.Fatal("one generic material word cannot justify a POD shirt suggestion")
	}
	if !matchesLeadProduct("Cần tìm hoodie", "Áo hoodie unisex") {
		t.Fatal("a distinctive product noun should match")
	}
}

func TestLeadBusinessIntent(t *testing.T) {
	if !wantsPersonalizedPOD("Cần in theo logo lên áo hoodie") {
		t.Fatal("custom logo should route to POD")
	}
	if !wantsBulkSourcing("Cần nhập hàng 300 hộp/tháng từ 1688") {
		t.Fatal("wholesale order should route to sourcing")
	}
	if !wantsBulkSourcing("Mua 300 hộp bổ khớp cho chó") {
		t.Fatal("explicit wholesale quantity should route to sourcing")
	}
	if wantsPersonalizedPOD("Cần nhập 300 hộp thực phẩm bổ khớp cho chó") {
		t.Fatal("ordinary bulk purchase should not route to POD")
	}
	if !wantsBulkSourcing("Looking for a European dropshipping supplier for this exact hand massager") {
		t.Fatal("English dropshipping supplier should route to sourcing")
	}
}

func TestShippingOnlyForExplicitUSOrdinaryApparel(t *testing.T) {
	if !isUSDestination("Cần gửi áo hoodie sang Mỹ") || isUSDestination("Cần gửi áo hoodie sang Anh") {
		t.Fatal("US destination must be explicit")
	}
	if !isOrdinaryApparel("Áo hoodie cotton") || isOrdinaryApparel("Thực phẩm bổ sung cho chó") {
		t.Fatal("restricted goods must not use ordinary-goods Epacket")
	}
}

func TestShippingRouteFactsFromLead(t *testing.T) {
	post := "Cần nhập viên bổ khớp cho chó từ 1688 về Mỹ khoảng 300 hộp/tháng"
	if got := leadQuantity(post); got != 300 {
		t.Fatalf("quantity=%d", got)
	}
	if got := leadDestinationCountry(post); got != "US" {
		t.Fatalf("destination=%q", got)
	}
	if got := leadQuantity("Cần gửi áo sang Mỹ"); got != 0 {
		t.Fatalf("unstated quantity must remain unknown, got %d", got)
	}
}

func TestSupplierPriceFreshnessRequiresCaptureDate(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	supplier := &models.SupplierMatch{Name: "Viên bổ khớp", URL: "https://detail.1688.com/offer/2.html", PriceText: "¥20", CapturedAt: "2026-09-29T00:00:00Z"}
	if !freshSupplierPrice(supplier, now) {
		t.Fatal("recent price should be eligible")
	}
	supplier.CapturedAt = "2026-09-09T00:00:00Z"
	if freshSupplierPrice(supplier, now) {
		t.Fatal("stale price should trigger fresh lookup")
	}
}

func TestSupplierFallbackReplyIncludesCapturedPriceAndPerParcelRate(t *testing.T) {
	supplier := &models.SupplierMatch{Name: "Áo hoodie", PriceText: "¥28.9", Shipping: &models.ShippingReference{
		PriceText: "$14.20", Transit: "6–12 ngày làm việc", Basis: "1 kiện; không phải tổng cước lô 300 sản phẩm",
	}}
	reply := supplierFallbackReply("Ngọc Trâm", supplier, "Cần nhập 300 áo hoodie về Mỹ")
	for _, fact := range []string{"Ngọc Trâm", "¥28.9", "$14.20/kiện", "chưa phải tổng cước lô", "6–12 ngày làm việc"} {
		if !strings.Contains(reply, fact) {
			t.Fatalf("reply missing %q: %s", fact, reply)
		}
	}
}

func TestSupplierFallbackReplyNamesRouteWithoutInventingRate(t *testing.T) {
	supplier := &models.SupplierMatch{Name: "Viên bổ khớp cho chó", PriceText: "¥20"}
	reply := supplierFallbackReply("Ngọc Trâm", supplier, "Cần nhập 300 hộp về Mỹ")
	if !strings.Contains(reply, "tuyến CN→US") || !strings.Contains(reply, "cước cần xác nhận") || strings.Contains(reply, "$14.20") {
		t.Fatalf("expected route and no invented rate: %s", reply)
	}
}

func TestBuildLeadSuggestionKeepsLiveSupplierWhenKnowledgeUnavailable(t *testing.T) {
	link := "https://detail.1688.com/offer/2.html"
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		return &ResolvedSupplier{Match: &models.SupplierMatch{
			Name: "Viên bổ khớp cho chó", URL: link, PriceText: "¥20",
		}}, nil
	}
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1,
		"Cần nhập 300 hộp viên bổ khớp cho chó từ 1688 về Mỹ", "Ngọc Trâm", nil, lookup)
	if result.Supplier == nil || result.Supplier.URL != link || !strings.Contains(result.Reply, "¥20") || !strings.Contains(result.Reply, "tuyến CN→US") {
		t.Fatalf("live supplier should survive unavailable knowledge: %+v", result)
	}
}
