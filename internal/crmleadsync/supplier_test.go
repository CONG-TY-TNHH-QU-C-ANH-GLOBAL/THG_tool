package crmleadsync

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
)

func TestPayloadFor_CarriesTheSupplierBlock(t *testing.T) {
	event := leadingest.LeadEvent{
		OrgID: 7, AuthorName: "Minh", PostURL: "https://facebook.com/groups/1/posts/2",
		OccurredAt: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC),
	}
	suggestion := models.LeadSuggestion{
		Reply: "Hi Minh, ...", ProductName: "Hoodie", ProductURL: "https://thgfulfill.com/p/hoodie",
		Supplier: &models.SupplierMatch{
			Platform: "1688", Name: "Áo hoodie nỉ bông", URL: "https://detail.1688.com/offer/1.html",
			PriceText: "¥28.9", WeightKG: "0.292 kg", MOQText: "1 件", ShipFrom: "广东省广州市",
		},
	}

	got, ok := payloadFor(event, suggestion)
	if !ok {
		t.Fatal("expected a payload")
	}
	if got.Enrichment.Supplier == nil {
		t.Fatal("supplier block must reach the CRM")
	}
	if got.Enrichment.Supplier.PriceText != "¥28.9" || got.Enrichment.Supplier.WeightText != "0.292 kg" {
		t.Errorf("supplier numbers not copied: %+v", got.Enrichment.Supplier)
	}

	// The supplier block must be a distinct key, never merged into the catalog
	// product fields — the CRM has to tell the two apart.
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"supplier":{`) {
		t.Errorf("expected a nested supplier object, got %s", encoded)
	}
}

func TestSupplierFrom_DropsPartialMatches(t *testing.T) {
	cases := map[string]*models.SupplierMatch{
		"nil":       nil,
		"no link":   {Platform: "1688", Name: "Áo hoodie", PriceText: "¥28.9"},
		"no name":   {Platform: "1688", URL: "https://detail.1688.com/offer/1.html"},
		"all empty": {},
	}
	for name, match := range cases {
		if got := supplierFrom(match); got != nil {
			t.Errorf("%s: expected nil, got %+v", name, got)
		}
	}
}

func TestPayloadFor_OmitsSupplierWhenAbsent(t *testing.T) {
	event := leadingest.LeadEvent{OrgID: 7, AuthorName: "Minh", PostURL: "https://facebook.com/p/1"}
	got, ok := payloadFor(event, models.LeadSuggestion{Reply: "Hi"})
	if !ok {
		t.Fatal("expected a payload")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), "supplier") {
		t.Errorf("absent supplier must not appear on the wire: %s", encoded)
	}
}
