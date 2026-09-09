package control

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// A match that carries numbers but no name or link is not an offer: rendering
// its price strip alone would show an orphan price with nothing to buy.
func TestSupplierSummary_GuardedMatchProducesNoOrphanNumbers(t *testing.T) {
	partial := &models.SupplierMatch{PriceText: "¥28.9", WeightKG: "0.292 kg"}
	if partial.HasOffer() {
		t.Fatal("a match without name and URL must not count as an offer")
	}
	if got := supplierSummary(&models.SupplierMatch{}); got != "" {
		t.Errorf("guarded empty match must summarize to nothing, got %q", got)
	}
}

func TestSupplierSummary_JoinsOnlyKnownFacts(t *testing.T) {
	got := supplierSummary(&models.SupplierMatch{
		Name: "Áo hoodie", URL: "https://detail.1688.com/offer/1.html",
		PriceText: "¥28.9", MOQText: "1 件", ShipFrom: "广东省广州市",
	})
	if strings.Contains(got, "kg") {
		t.Errorf("an unknown weight must be omitted, got %q", got)
	}
	if got != "¥28.9 · MOQ 1 件 · 广东省广州市" {
		t.Errorf("summary = %q", got)
	}
}
