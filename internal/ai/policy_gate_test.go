package ai

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

func gateDecision(conf float64, gap bool, products, caps int) *models.CommentDecision {
	d := &models.CommentDecision{Intent: "product_seeking", Confidence: conf, KnowledgeGap: gap}
	for i := 0; i < products; i++ {
		d.Selected.Products = append(d.Selected.Products,
			models.GroundedItem{Label: "Túi zip", SKU: "ZIP-01", PriceText: "12.000đ", Score: conf})
	}
	for i := 0; i < caps; i++ {
		d.Selected.Capabilities = append(d.Selected.Capabilities,
			models.GroundedItem{Label: "Sourcing + fulfill", PriceText: "từ 3 USD", Score: conf})
	}
	return d
}

// High confidence + policy allows → product name AND price survive.
func TestGate_HighConfidence_PriceAllowedByPolicy(t *testing.T) {
	p := DefaultCommentPolicies() // allow price, min conf 0.7
	d := ApplyGate(gateDecision(0.9, false, 1, 0), EvaluateGate(gateDecision(0.9, false, 1, 0), p))
	if d.KnowledgeGap || len(d.Selected.Products) != 1 || d.Selected.Products[0].PriceText != "12.000đ" {
		t.Fatalf("high-conf+allowed must keep product+price: %+v", d.Selected)
	}
}

// High confidence but allow_price_in_comments=false → product name
// only, price stripped everywhere.
func TestGate_HighConfidence_PricePolicyFalse(t *testing.T) {
	p := DefaultCommentPolicies()
	p.AllowPriceInComments = false
	in := gateDecision(0.9, false, 1, 1)
	d := ApplyGate(in, EvaluateGate(in, p))
	if len(d.Selected.Products) != 1 || d.Selected.Products[0].PriceText != "" || d.Selected.Capabilities[0].PriceText != "" {
		t.Fatalf("price must be stripped when policy forbids: %+v", d.Selected)
	}
	if d.Selected.Products[0].SKU != "ZIP-01" {
		t.Fatalf("product identity must survive price stripping")
	}
	// The original decision is never mutated (subtractive copy).
	if in.Selected.Products[0].PriceText != "12.000đ" {
		t.Fatalf("ApplyGate mutated its input")
	}
}

// Medium confidence → category/service mention only: products dropped,
// no exact price; with no capabilities left it degrades to generic.
func TestGate_MediumConfidence_NoExactPrice(t *testing.T) {
	p := DefaultCommentPolicies()
	withService := gateDecision(0.5, false, 1, 1)
	d := ApplyGate(withService, EvaluateGate(withService, p))
	if len(d.Selected.Products) != 0 || d.KnowledgeGap {
		t.Fatalf("medium conf must drop products but keep service mention: %+v", d)
	}
	if d.Selected.Capabilities[0].PriceText != "" {
		t.Fatalf("medium conf must not quote exact price")
	}
	productOnly := gateDecision(0.5, false, 1, 0)
	d = ApplyGate(productOnly, EvaluateGate(productOnly, p))
	if !d.KnowledgeGap {
		t.Fatalf("medium conf with nothing but products must degrade to generic")
	}
}

// Low/no confidence and knowledge gaps → generic comment, never an
// invented product/price (catalog connected but no match).
func TestGate_NoMatch_NoHallucination(t *testing.T) {
	p := DefaultCommentPolicies()
	gap := &models.CommentDecision{Intent: "ambiguous", Confidence: 0, KnowledgeGap: true}
	d := ApplyGate(gap, EvaluateGate(gap, p))
	if !d.KnowledgeGap || d.Selected.HasOffer() {
		t.Fatalf("no-match must stay generic: %+v", d)
	}
	if v := EvaluateGate(nil, p); v.Mode != GateGeneric {
		t.Fatalf("nil decision must be generic")
	}
}

// Custom threshold from policy keys is honored.
func TestGate_ConfidenceThreshold(t *testing.T) {
	p := DefaultCommentPolicies()
	p.MinConfidenceToQuotePrice = 0.9
	in := gateDecision(0.8, false, 1, 1)
	if v := EvaluateGate(in, p); v.Mode != GateCategoryMention {
		t.Fatalf("0.8 < 0.9 threshold must be category mention, got %s", v.Mode)
	}
	p.MinConfidenceToQuotePrice = 0.7
	if v := EvaluateGate(in, p); v.Mode != GateProductPitch || !v.AllowPrice {
		t.Fatalf("0.8 >= 0.7 must be product pitch with price")
	}
}
