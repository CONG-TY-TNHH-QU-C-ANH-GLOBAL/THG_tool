package facebook

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestSupplierLookupRejectsLinkedProductWithDifferentID(t *testing.T) {
	price := 20.0
	client := &fakeSupplierReader{product: &suppliersourcing.Product{
		Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/3.html", Price: &price,
	}}
	got, err := NewSupplierLookup(client)(context.Background(),
		"Cần nhập viên bổ khớp cho chó https://detail.1688.com/offer/2.html")
	if err == nil || got != nil {
		t.Fatalf("pasted listing must not be called exact when detail resolves elsewhere: %+v, %v", got, err)
	}
	if !sameMarketplaceListing("https://m.1688.com/offer/2.html?from=share", "https://detail.1688.com/offer/2.html") {
		t.Fatal("mobile and desktop links with one offer ID should match")
	}
}

func TestSupplierQueryFindsPODBlankWhenCatalogMisses(t *testing.T) {
	for _, post := range []string{
		"Cần in logo lên 300 áo hoodie đi Mỹ",
		"Cần áo hoodie in logo công ty",
		"Cần mua 300 áo hoodie in logo công ty",
	} {
		if got := supplierQuery(post); got != "áo hoodie" {
			t.Fatalf("POD product should be searchable: %q => %q", post, got)
		}
	}
	if got := supplierQuery("Cần in logo lên sản phẩm theo thiết kế"); got != "" {
		t.Fatalf("unspecified POD product should not spend Elim quota: %q", got)
	}
	if got := supplierQuery("Need to print logo on 300 hoodies"); got != "hoodies" || supplierQueryLanguage("Need to print logo on 300 hoodies") != "en" {
		t.Fatalf("English POD blank should use an English marketplace query, got %q", got)
	}
}

func TestSupplierLookupSkipsUnusableFirstDetailWithinBudget(t *testing.T) {
	price := 24.0
	client := &fakeSupplierReader{
		items: []suppliersourcing.SearchItem{
			{ID: "1", Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/1.html"},
			{ID: "2", Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/2.html"},
		},
		detailFn: func(id string) (*suppliersourcing.Product, error) {
			if id == "1" {
				return &suppliersourcing.Product{Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/1.html"}, nil
			}
			return &suppliersourcing.Product{Title: "Viên bổ khớp cho chó", Link: "https://detail.1688.com/offer/2.html", Price: &price}, nil
		},
	}
	got, err := NewSupplierLookup(client)(context.Background(), "Cần nhập viên bổ khớp cho chó từ 1688")
	if err != nil || got == nil || got.Match.URL != "https://detail.1688.com/offer/2.html" || client.details != 2 {
		t.Fatalf("second usable offer should be selected within detail budget: got=%+v details=%d err=%v", got, client.details, err)
	}
}

func TestSupplierQueryRejectsWrongProductQualifier(t *testing.T) {
	if matchesSupplierQuery("viên bổ khớp cho chó", "Viên bổ khớp cho mèo") {
		t.Fatal("supplier title must retain the target animal")
	}
	if !matchesSupplierQuery("viên bổ khớp cho chó", "Thực phẩm bổ sung dinh dưỡng khớp và hông chó") {
		t.Fatal("product wording may vary while identity remains")
	}
}
