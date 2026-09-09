package facebook

import (
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

func floatPtr(v float64) *float64 { return &v }
func intPtr(v int) *int           { return &v }

func supplierCandidate() models.KnowledgeCandidate {
	return models.KnowledgeCandidate{
		Kind:      string(assets.AssetSupplierProduct),
		Title:     "Áo thun cotton 270g",
		SourceURL: "https://detail.1688.com/offer/795570801986.html",
		ImageURL:  "https://cbu01.alicdn.com/img/a.jpg",
		Supplier: &models.SupplierFacts{
			Platform: "alibaba", ProductID: "795570801986",
			PriceCNY: floatPtr(28.9), MOQ: intPtr(1), Unit: "件",
			WeightKG: floatPtr(0.292), ShipFrom: "广东省广州市", ShopName: "广州市潮元素服装厂",
			CapturedAt: time.Date(2026, 9, 9, 3, 0, 0, 0, time.UTC),
		},
	}
}

func TestPickSuggestedSupplier_MapsFactsAndLabelsMarketplace(t *testing.T) {
	got := PickSuggestedSupplier([]models.KnowledgeCandidate{supplierCandidate()})
	if !got.HasOffer() {
		t.Fatalf("expected an offer, got %+v", got)
	}
	if got.Platform != "1688" {
		t.Errorf("platform = %q, want the operator-facing 1688 label", got.Platform)
	}
	if got.PriceText != "¥28.9" {
		t.Errorf("price = %q, want ¥28.9", got.PriceText)
	}
	if got.WeightKG != "0.292 kg" {
		t.Errorf("weight = %q, want 0.292 kg", got.WeightKG)
	}
	if got.MOQText != "1 件" {
		t.Errorf("moq = %q, want the marketplace unit word", got.MOQText)
	}
	if got.CapturedAt != "2026-09-09T03:00:00Z" {
		t.Errorf("capturedAt = %q", got.CapturedAt)
	}
}

func TestPickSuggestedSupplier_RejectsNonHTTPSAndWrongKind(t *testing.T) {
	insecure := supplierCandidate()
	insecure.SourceURL = "http://detail.1688.com/offer/1.html"
	if got := PickSuggestedSupplier([]models.KnowledgeCandidate{insecure}); got != nil {
		t.Errorf("a non-https link must not become an offer, got %+v", got)
	}

	catalog := supplierCandidate()
	catalog.Kind = string(assets.AssetPODProduct)
	if got := PickSuggestedSupplier([]models.KnowledgeCandidate{catalog}); got != nil {
		t.Errorf("a catalog product must never be picked as a supplier, got %+v", got)
	}
}

func TestPickSuggestedSupplier_TakesFirstRankedEligible(t *testing.T) {
	unusable := supplierCandidate()
	unusable.SourceURL = ""
	second := supplierCandidate()
	second.Title = "Second"
	got := PickSuggestedSupplier([]models.KnowledgeCandidate{unusable, second})
	if got == nil || got.Name != "Second" {
		t.Fatalf("expected the first candidate with a usable link, got %+v", got)
	}
}

func TestBuildGroundedFacts_OmitsWhatTheMarketplaceDidNotPublish(t *testing.T) {
	candidate := supplierCandidate()
	candidate.Supplier.WeightKG = nil
	supplier := PickSuggestedSupplier([]models.KnowledgeCandidate{candidate})

	facts := BuildGroundedFacts(SuggestedProduct{Name: "Hoodie THG", URL: "https://thgfulfill.com/p/hoodie"}, supplier)

	for _, want := range []string{"OUR CATALOG PRODUCT", "Hoodie THG", "SOURCEABLE ITEM", "¥28.9", "minimum order: 1 件"} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts block missing %q:\n%s", want, facts)
		}
	}
	if strings.Contains(facts, "shipping weight") {
		t.Errorf("an unknown weight must be omitted entirely:\n%s", facts)
	}
}

func TestBuildGroundedFacts_EmptyWhenNothingGrounded(t *testing.T) {
	if facts := BuildGroundedFacts(SuggestedProduct{}, nil); facts != "" {
		t.Errorf("no grounding must produce no facts, got %q", facts)
	}
}
