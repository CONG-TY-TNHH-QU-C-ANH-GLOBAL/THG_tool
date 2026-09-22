package ai

import (
	"strings"
	"testing"
)

// TestProductFacts_Block pins the prompt section that carries catalog numbers.
//
// The regression this guards: retrieval computed PriceText and then dropped it,
// so every suggested reply stayed vague about price. An empty block must render
// as "" so callers that ground nothing keep the original prompt shape.
func TestProductFacts_Block(t *testing.T) {
	if got := (ProductFacts{}).block(); got != "" {
		t.Fatalf("empty facts should render nothing, got %q", got)
	}
	if got := (ProductFacts{Name: "   ", PriceText: "\t"}).block(); got != "" {
		t.Fatalf("whitespace-only facts should render nothing, got %q", got)
	}

	got := ProductFacts{Name: "Hoodie 350gsm", PriceText: "4.5-9 USD", URL: "https://x.test/p/1"}.block()
	for _, want := range []string{
		"PRODUCT FACTS (copy these numbers EXACTLY — never round, never invent a price):",
		"\n- Product: Hoodie 350gsm",
		"\n- Price: 4.5-9 USD",
		"\n- Link: https://x.test/p/1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("block missing %q\ngot: %q", want, got)
		}
	}

	// A product with no stored price must still ground name and link, and must
	// not emit an empty "Price:" line for the model to fill in itself.
	noPrice := ProductFacts{Name: "Tee", URL: "https://x.test/p/2"}.block()
	if strings.Contains(noPrice, "Price:") {
		t.Fatalf("missing price must not render a Price line, got %q", noPrice)
	}
	if !strings.Contains(noPrice, "- Product: Tee") {
		t.Fatalf("name should still be grounded, got %q", noPrice)
	}
}
