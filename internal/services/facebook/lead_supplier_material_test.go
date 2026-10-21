package facebook

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestSupplierCottonRequestIsSearchable(t *testing.T) {
	for _, post := range []string{
		"Looking for a supplier for cotton bags, 100 pcs to the US",
		"Cần mua 100 cotton bags đi Mỹ",
	} {
		if got := supplierQuery(post); got != "cotton bags" {
			t.Fatalf("material qualifier made a concrete item unsearchable: %q", got)
		}
	}
}

func TestSupplierCottonQualifierCannotMatchPolyester(t *testing.T) {
	if matchesSupplierQuery("cotton tote bags", "polyester tote bags") {
		t.Fatal("source selection dropped requested material")
	}
	if !matchesSupplierQuery("cotton tote bags", "large cotton tote bags") {
		t.Fatal("matching material and product should remain eligible")
	}
}

func TestSupplierMaterialRecheckedAtDetail(t *testing.T) {
	price := 20.0
	client := &fakeSupplierReader{
		items: []suppliersourcing.SearchItem{
			{ID: "1", Title: "Cotton tote bags", Link: "https://detail.1688.com/offer/1.html"},
			{ID: "2", Title: "Cotton tote bags", Link: "https://detail.1688.com/offer/2.html"},
		},
		detailFn: func(id string) (*suppliersourcing.Product, error) {
			title := "Polyester tote bags"
			if id == "2" {
				title = "Cotton tote bags"
			}
			return &suppliersourcing.Product{ID: id, Title: title,
				Link: "https://detail.1688.com/offer/" + id + ".html", Price: &price}, nil
		},
	}
	got, err := NewSupplierLookup(client)(context.Background(), "Need a supplier for cotton tote bags from 1688")
	if err != nil || got == nil || got.Match.URL != "https://detail.1688.com/offer/2.html" || client.details != 2 {
		t.Fatalf("detail changed material without rejection: details=%d err=%v", client.details, err)
	}
}
