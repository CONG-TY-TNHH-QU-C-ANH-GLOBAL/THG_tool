package facebook

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestSupplierRewritePreservesParcelAndPODQualifications(t *testing.T) {
	supplier := &models.SupplierMatch{Name: "cotton bags", URL: "https://detail.1688.com/offer/123.html", Similar: true,
		PriceText: "20 CNY", MOQText: "2 pcs", Shipping: &models.ShippingReference{PriceText: "$14.20", Basis: "không phải tổng cước lô"}}
	post := "Need custom logo on cotton bags, 300 pcs to the US, European supplier"
	want := "China alternative, similar cotton bags: reference price 20 CNY, MOQ 2 pcs; reference shipping $14.20 per parcel (not the lot total). We need to confirm customization. " + supplier.URL
	if !safeSupplierReply(want, post, supplier, true) {
		t.Fatal("grounded qualifications should pass")
	}
	for _, changed := range []string{
		strings.Replace(want, "not the lot total", "lot total", 1),
		strings.Replace(want, "per parcel", "for the order", 1),
		strings.Replace(want, "confirm customization", "start production", 1),
		strings.Replace(want, "alternative", "supplier", 1),
		strings.Replace(want, "$14.20", "$14.25", 1),
		strings.Replace(want, "similar", "similar exact same", 1),
		want + " Additional fee $300.",
		want + " https://unrelated.example/buy",
	} {
		if safeSupplierReply(changed, post, supplier, true) {
			t.Fatalf("unsafe qualification accepted: %q", changed)
		}
	}
}

func TestShortImageDependentPostDoesNotAbsorbComments(t *testing.T) {
	post := "Facebook Lan 2 phút trước · Need this Xem bản dịch 3 1 Seller · We offer warehouse services"
	if got := leadPostBody(post); got != "Need this" {
		t.Fatalf("comment/UI contaminated short post: %q", got)
	}
}
