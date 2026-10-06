package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestThirdReviewCatalogHeadPhraseIgnoresQualifiers(t *testing.T) {
	catalog := []models.KnowledgeCandidate{
		{Kind: "POD_product", Title: "Áo thun cao cấp in logo", SourceURL: "https://shop.example/ao"},
		{Kind: "POD_product", Title: "Cốc sứ in logo theo yêu cầu", SourceURL: "https://shop.example/coc"},
	}
	got := PickSuggestedProductDetails(matchingCandidates("Cần in logo lên cốc sứ cao cấp cho công ty", catalog))
	if got.Name != "Cốc sứ in logo theo yêu cầu" {
		t.Fatalf("mug lead must not be offered a T-shirt through %q: got %q", "cao cấp", got.Name)
	}
	if matchesLeadProduct("Cần in logo làm quà tặng cho khách", "Quà tặng sổ tay in logo") {
		t.Fatal("a shared use word such as quà tặng is not a product match")
	}
}

func TestThirdReviewQueryDropsTrailingDeliveryAndContactWords(t *testing.T) {
	for post, want := range map[string]string{
		"Cần nhập 300 hộp viên bổ khớp cho chó gửi về Mỹ": "viên bổ khớp cho chó",
		"Cần in logo lên túi vải canvas gửi về Mỹ":        "túi vải canvas",
		"Cần mua áo hoodie nỉ địa chỉ 12 Lê Lợi quận 1":   "áo hoodie nỉ",
		"Cần mua áo hoodie nỉ em Hoa nhé":                 "áo hoodie nỉ",
		"Cần mua áo hoodie nỉ fb.com/hoa.nguyen":          "áo hoodie nỉ",
	} {
		if got := supplierQuery(post); got != want {
			t.Errorf("supplierQuery(%q) = %q, want %q", post, got, want)
		}
	}
}

func TestThirdReviewPODAlternativeFoundWhenPostSaysGuiVeMy(t *testing.T) {
	price := 9.0
	link := "https://detail.1688.com/offer/9.html"
	reader := &fakeSupplierReader{
		items:   []suppliersourcing.SearchItem{{ID: "9", Title: "Túi vải canvas trơn", Link: link}},
		product: &suppliersourcing.Product{Platform: "alibaba", ID: "9", Title: "Túi vải canvas trơn", Link: link, Price: &price},
	}
	got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, "Cần in logo lên 200 túi vải canvas gửi về Mỹ", "Lan", nil, NewSupplierLookup(reader))
	if !got.Supplier.HasOffer() || !strings.Contains(got.Reply, "nguồn sản phẩm thay thế") || len(reader.queries) != 1 {
		t.Fatalf("POD alternative must be found in one search and labelled: %+v queries=%v", got, reader.queries)
	}
}

func TestThirdReviewPoweredClothingIsNotStandardCargo(t *testing.T) {
	for _, name := range []string{"Áo khoác có quạt làm mát", "Áo thun phát sáng", "LED light up hoodie"} {
		if isOrdinaryApparel(name) {
			t.Errorf("%q may contain a battery and must not get the ordinary-goods rate", name)
		}
	}
	if !isOrdinaryApparel("Áo hoodie nỉ") {
		t.Fatal("plain hoodie should remain standard cargo")
	}
}

// Warehousing is a THG service: the draft offers it instead of asking for a product model.
func TestThirdReviewWarehouseQuestionsGetNoSourcingDraft(t *testing.T) {
	for post, want := range map[string]string{
		"Need a 3PL warehouse in the US for my Shopify store": "warehousing and fulfillment",
		"Cần tìm kho Mỹ nhận hàng nhập từ 1688":               "dịch vụ kho và fulfillment",
	} {
		got := noOfferSuggestionForLookup(post, "", lookupCompleted)
		if !strings.Contains(got.Reply, want) || got.SourcingNote != "" || strings.Contains(got.Reply, "mẫu") || strings.Contains(got.Reply, "model") {
			t.Errorf("warehouse question %q must get a warehousing draft, not a sourcing one: %+v", post, got)
		}
	}
}

func TestThirdReviewEnglishPostWithoutQueryGetsEnglishDraft(t *testing.T) {
	got := noOfferSuggestionForLookup("Looking for a supplier of blenders, 500 units to the US", "", lookupCompleted)
	if !strings.HasPrefix(got.Reply, "Hi there") {
		t.Fatalf("English post must get an English draft: %q", got.Reply)
	}
}

func TestThirdReviewEnglishQueryInVietnamesePostSearchesInEnglish(t *testing.T) {
	reader := &fakeSupplierReader{}
	_, _ = NewSupplierLookup(reader)(context.Background(), "Cần nhập 300 cái hand massager ship to US")
	if len(reader.langs) == 0 || reader.langs[0] != "en" {
		t.Fatalf("an English product phrase must request English titles: %v", reader.langs)
	}
}

func TestThirdReviewBudgetSkipIsReportedAsTimeout(t *testing.T) {
	status := classifySupplierLookupError(errSupplierBudget, false)
	if note := noOfferSuggestionForLookup("Cần nhập viên bổ khớp cho chó", "", status).SourcingNote; !strings.Contains(note, "hết thời gian") {
		t.Fatalf("a skipped lookup must say it ran out of time: %q", note)
	}
}

func TestThirdReviewShortLinkRequiresResolvedItemID(t *testing.T) {
	if linkedMarketplaceListing("https://qr.1688.com/s/abc", "https://detail.1688.com/offer/undefined.html") {
		t.Fatal("a resolved link without an item ID must not be shown as the lead's listing")
	}
	if !linkedMarketplaceListing("https://qr.1688.com/s/abc", "https://detail.1688.com/offer/123.html") {
		t.Fatal("a resolved 1688 item should be accepted")
	}
}
