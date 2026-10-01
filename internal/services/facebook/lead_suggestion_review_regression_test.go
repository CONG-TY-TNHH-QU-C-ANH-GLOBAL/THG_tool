package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestReviewCatalogRequiresProductIdentity(t *testing.T) {
	post := "Personalized Christmas stockings for Etsy POD sellers"
	if matchesLeadProduct(post, "Personalized Christmas T-Shirt") {
		t.Fatal("seasonal and personalization words must not turn a stocking lead into a shirt")
	}
	if !matchesLeadProduct(post, "Custom Name Christmas Stocking") {
		t.Fatal("stockings in the post should match a stocking catalog item")
	}
}

func TestReviewIntentRequiresWholeWords(t *testing.T) {
	for _, post := range []string{
		"Cần tìm xưởng làm tripod số lượng lớn",
		"Mình bán podcast mic, cần nhập 200 cái",
	} {
		if wantsPersonalizedPOD(post) {
			t.Fatalf("ordinary product contains POD letters: %q", post)
		}
	}
	if !wantsPersonalizedPOD("Cần in logo lên áo hoodie") {
		t.Fatal("explicit logo printing must remain POD")
	}
	if !wantsPersonalizedPOD("Need to print logo on 300 hoodies") {
		t.Fatal("English logo printing must route to POD")
	}
}

func TestReviewDestinationRequiresCountryPhrase(t *testing.T) {
	for _, post := range []string{
		"Cần nhập 500 hộp kẹo táo, gửi về anh Nam ở Hà Nội",
		"How to use this hand massager?",
		"Can you send the sample to us?",
	} {
		if got := leadDestinationCountry(post); got != "" {
			t.Fatalf("person or English verb mistaken for a country: %q => %q", post, got)
		}
	}
	if got := leadDestinationCountry("Cần gửi áo hoodie sang Mỹ"); got != "US" {
		t.Fatalf("explicit US destination lost: %q", got)
	}
	if got := leadDestinationCountry("Ship this hoodie to US"); got != "US" {
		t.Fatalf("explicit uppercase US destination lost: %q", got)
	}
	if got := leadDestinationCountry("Cần gửi áo hoodie đi nước Anh"); got != "GB" {
		t.Fatalf("explicit UK destination lost: %q", got)
	}
}

func TestReviewApparelRequiresWholeProductWords(t *testing.T) {
	for _, product := range []string{"Kẹo táo", "Nước ép táo", "Tất cả sản phẩm", "Shipping address"} {
		if isOrdinaryApparel(product) {
			t.Fatalf("non-apparel product got ordinary apparel rate: %q", product)
		}
	}
	if !isOrdinaryApparel("Áo hoodie cotton") {
		t.Fatal("ordinary hoodie must remain eligible for standard apparel rate")
	}
}

func TestReviewQuantityOnlyPhraseDoesNotSearchMarketplace(t *testing.T) {
	if got := supplierQuery("Mình bán podcast mic, cần nhập 200 cái"); got != "" {
		t.Fatalf("quantity without a product must not spend an Elim request: %q", got)
	}
}

func TestReviewPODWithoutCatalogLabelsMarketplaceAlternative(t *testing.T) {
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		return &ResolvedSupplier{Match: &models.SupplierMatch{
			Name: "Áo hoodie trơn", URL: "https://detail.1688.com/offer/2.html", PriceText: "¥20", Similar: true,
		}}, nil
	}
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1,
		"Cần mua 300 áo hoodie in logo công ty từ 1688", "Lan", nil, lookup)
	if result.Supplier == nil || !strings.Contains(result.Reply, "nguồn sản phẩm thay thế") ||
		!strings.Contains(result.Reply, "xác nhận khả năng in") {
		t.Fatalf("POD source alternative must be clearly qualified: %+v", result)
	}
}

func TestReviewUnsafeQuoteInputs(t *testing.T) {
	cases := []struct{ name, post, product string }{
		{"person name is not UK", "Cần nhập 500 hộp kẹo táo, gửi về anh Nam ở Hà Nội", "Kẹo táo"},
		{"five products are not one parcel", "Cần mua 5 áo hoodie cotton gửi sang Mỹ", "Áo hoodie cotton"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			weight := 0.4
			called := false
			lookup := func(context.Context, string) (*ResolvedSupplier, error) {
				return &ResolvedSupplier{Match: &models.SupplierMatch{
					Name: tc.product, URL: "https://detail.1688.com/offer/2.html", PriceText: "¥20",
				}, WeightKG: &weight}, nil
			}
			quote := func(context.Context, models.ShippingRequest) (*models.ShippingReference, error) {
				called = true
				return &models.ShippingReference{PriceText: "$14.20"}, nil
			}
			result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, tc.post, "Lan", quote, lookup)
			if called || result.Supplier == nil || result.Supplier.Shipping != nil || strings.Contains(result.Reply, "$14.20") {
				t.Fatalf("unsafe shipping facts produced a number: %+v", result)
			}
		})
	}
}
