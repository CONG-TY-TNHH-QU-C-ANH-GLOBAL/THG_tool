package facebook

import (
	"context"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestFusionSupplierQueryKeepsProductWithoutPerson(t *testing.T) {
	for post, want := range map[string]string{
		"Cần mua áo hoodie nỉ Nguyễn Thị Hoa đặt": "áo hoodie nỉ",
		"Cần mua áo hoodie nỉ Hoa đặt":            "áo hoodie nỉ",
		"Cần mua Áo Hoodie Nguyễn Hoa":            "áo hoodie",
		"Cần nhập Viên Bổ Khớp Cho Chó":           "viên bổ khớp cho chó",
		"Cần mua Nike Air Max":                    "nike air max",
	} {
		if got := supplierQuery(post); got != want {
			t.Errorf("supplierQuery(%q) = %q, want %q", post, got, want)
		}
	}
	if got := supplierQuery("Cần mua áo hoodie nỉ nguyễn thị hoa"); got != "" {
		t.Fatalf("ambiguous lower-case personal name must skip external search: %q", got)
	}
}

func TestFusionEnglishSupplierQueryDropsAppendedPerson(t *testing.T) {
	if got := supplierQuery("Looking for a supplier for red shirt John Smith"); got != "red shirt" {
		t.Fatalf("English customer name must not be sent as an Elim query: %q", got)
	}
}

func TestFusionLanguageDoesNotTreatUnaccentedVietnameseAsEnglish(t *testing.T) {
	if leadLooksEnglish("Can nhap 300 ao hoodie gui ve My") {
		t.Fatal("unaccented Vietnamese post must not get an English draft")
	}
	if asciiQuery("ao hoodie") {
		t.Fatal("unaccented Vietnamese product must not request English listing titles")
	}
	if !asciiQuery("hand massager") {
		t.Fatal("English product terms must retain English listing titles")
	}
}

func TestFusionVolumeTierPriceUsesLeadQuantity(t *testing.T) {
	first, second := 20.0, 16.0
	product := &suppliersourcing.Product{
		QuoteType: "by_volume", Price: &first,
		PriceRange: []suppliersourcing.PriceTier{{MOQ: 2, Price: &first}, {MOQ: 300, Price: &second}},
	}
	if got := supplierSourcePriceText(product, suppliersourcing.PlatformAlibaba, 300, "vi"); !strings.Contains(got, "¥16") || !strings.Contains(got, "bậc 300+") {
		t.Fatalf("300 units should use the 300+ tier: %q", got)
	}
	if got := supplierSourcePriceText(product, suppliersourcing.PlatformAlibaba, 1, "vi"); !strings.Contains(got, "chưa áp dụng") {
		t.Fatalf("quantity below MOQ must not be represented as eligible: %q", got)
	}
	if got := supplierSourcePriceText(product, suppliersourcing.PlatformAlibaba, 300, "en"); !strings.Contains(got, "tier 300+") || strings.Contains(got, "giá bậc") {
		t.Fatalf("English draft must label the selected tier in English: %q", got)
	}
}

func TestFusionBulkOfferWithoutTiersLabelsUnconfirmedPrice(t *testing.T) {
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		return &ResolvedSupplier{Match: &models.SupplierMatch{
			Platform: "1688", Name: "Viên bổ khớp cho chó", URL: "https://detail.1688.com/offer/123.html", PriceText: "¥20",
		}}, nil
	}
	got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, "Cần nhập 300 hộp viên bổ khớp cho chó gửi về Mỹ", "Lan", nil, lookup)
	if !strings.Contains(got.Reply, "giá theo số lượng cần xác nhận") {
		t.Fatalf("bulk reference price needs a volume caveat: %q", got.Reply)
	}
}

func TestFusionPODDraftRejectsUnsupportedPriceAndLink(t *testing.T) {
	link := "https://thgfulfill.com/vi/catalog?productId=123"
	if generatedPODReplyGrounded("Áo này giá 139.000 VND. "+link, link) {
		t.Fatal("POD generator cannot invent a price absent from catalog facts")
	}
	if generatedPODReplyGrounded("Áo này 139.000đ. "+link, link) {
		t.Fatal("POD generator cannot invent an unverified VND price")
	}
	if generatedPODReplyGrounded("Mẫu áo: https://example.com/other", link) {
		t.Fatal("POD generator cannot substitute an unrelated link")
	}
	if !generatedPODReplyGrounded("Bên mình có mẫu áo này. "+link, link) {
		t.Fatal("grounded POD draft should remain available")
	}
}
