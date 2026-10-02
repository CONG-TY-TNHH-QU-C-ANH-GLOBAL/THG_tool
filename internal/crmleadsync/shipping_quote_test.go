package crmleadsync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestQuoteSupplierShipment_UsesAuthenticatedCRMRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("x-thg-integration-key") != "test-key" {
			t.Errorf("unexpected request: %s %q", r.Method, r.Header.Get("x-thg-integration-key"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["destinationCountry"] != "US" || body["quantity"] != float64(1) {
			t.Errorf("missing route facts: %+v, %v", body, err)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"lane":"international","currency":"USD","totalUsd":14.2,"billableKg":0.4,"transit":"6–12 ngày làm việc","source":"https://thgfulfill.com/vi/international-pricing"}`))
	}))
	defer server.Close()
	got, err := QuoteSupplierShipment(context.Background(), server.URL, "test-key", models.ShippingRequest{
		OriginCountry: "CN", DestinationCountry: "US", Quantity: 1, ShipmentMode: "parcel", CargoCategory: "standard", WeightKG: 0.4,
	})
	if err != nil || got.PriceText != "$14.20" || got.Transit == "" || got.SourceURL == "" {
		t.Fatalf("expected grounded reference, got %+v, %v", got, err)
	}
}

func TestQuoteSupplierShipment_RejectsUnusableRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer server.Close()
	if got, err := QuoteSupplierShipment(context.Background(), server.URL, "test-key", models.ShippingRequest{OriginCountry: "CN", DestinationCountry: "US", Quantity: 1, ShipmentMode: "parcel", CargoCategory: "standard", WeightKG: 0.4}); err == nil || got != nil {
		t.Fatalf("missing rate must be omitted, got %+v, %v", got, err)
	}
}

func TestQuoteSupplierShipment_BulkReferenceIsPerItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"lane":"international","currency":"USD","totalUsd":14.2,"billableKg":0.4,"transit":"6–12 ngày","source":"https://thgfulfill.com/vi/international-pricing"}`))
	}))
	defer server.Close()
	got, err := QuoteSupplierShipment(context.Background(), server.URL, "test-key", models.ShippingRequest{
		OriginCountry: "CN", DestinationCountry: "US", Quantity: 300, ShipmentMode: "bulk", CargoCategory: "standard", WeightKG: 0.4,
	})
	if err != nil || got == nil || !strings.Contains(got.Basis, "1 sản phẩm") || !strings.Contains(got.Basis, "không phải tổng cước lô 300") {
		t.Fatalf("bulk unit reference must not look like a lot price: %+v, %v", got, err)
	}
}
