package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

func TestEnglishEuropeanLeadOffersHonestAlternativeWithoutInventedShipping(t *testing.T) {
	link := "https://detail.1688.com/offer/12.html"
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		return &ResolvedSupplier{Match: &models.SupplierMatch{
			Name: "Electric hand massager", URL: link, PriceText: "¥42", Platform: "1688",
		}}, nil
	}
	post := "Hi everyone! I'm looking for a European dropshipping supplier for this exact hand massager or a very similar model."
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Oksana", nil, lookup)
	if result.Supplier == nil || result.Supplier.URL != link || !strings.Contains(result.Reply, "¥42") ||
		!strings.Contains(result.Reply, "China-based alternative") || !strings.Contains(result.Reply, "quantity and destination") ||
		strings.Contains(result.Reply, "reference shipping") {
		t.Fatalf("English alternative must be sourced and honest: %+v", result)
	}
}

func TestEnglishLeadThroughPricingHubUsesEnglishOnBothRequests(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["lang"] != "en" {
			t.Errorf("request %d did not ask for English: %v, %v", requests, body, err)
		}
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/api/scrape/search" {
			_, _ = w.Write([]byte(`{"ok":true,"items":[{"id":"12","title":"Electric hand massager","link":"https://detail.1688.com/offer/12.html"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"ok":true,"product":{"id":"12","title":"Electric hand massager","link":"https://detail.1688.com/offer/12.html","price":42}}`))
		}
	}))
	defer server.Close()
	client := suppliersourcing.NewClient(server.URL, "test-key", 2*time.Second)
	post := "Looking for a European dropshipping supplier for this exact hand massager or a very similar model."
	result := BuildLeadSuggestion(context.Background(), nil, nil, nil, 1, post, "Oksana", nil, NewSupplierLookup(client))
	if requests != 2 || result.Supplier == nil || !strings.Contains(result.Reply, "¥42") {
		t.Fatalf("end-to-end sourcing failed: requests=%d result=%+v", requests, result)
	}
}

func TestEnglishBulkRateIsLabelledPerParcel(t *testing.T) {
	supplier := &models.SupplierMatch{Name: "Hand massager", PriceText: "¥42", Shipping: &models.ShippingReference{
		PriceText: "$14.20", Basis: "1 kiện; không phải tổng cước lô 300 sản phẩm",
	}}
	reply := supplierFallbackReply("Oksana", supplier, "Looking for a dropshipping supplier for this hand massager to US")
	if !strings.Contains(reply, "$14.20 per parcel (not the total bulk shipping cost)") ||
		!strings.Contains(reply, "quantity and parcel details") {
		t.Fatalf("rate scope or missing facts unclear: %s", reply)
	}
}
