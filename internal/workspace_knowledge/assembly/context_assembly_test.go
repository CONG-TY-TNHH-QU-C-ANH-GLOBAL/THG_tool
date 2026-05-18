package assembly

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

func hit(typ assets.AssetType, title, desc string, payload string, score float64) retrieval.Hit {
	a := &assets.Asset{
		Type:        typ,
		Title:       title,
		Description: desc,
	}
	if payload != "" {
		a.Payload = json.RawMessage(payload)
	}
	return retrieval.Hit{Asset: a, Score: score, Reason: "test"}
}

func TestAssembleContext_GroupsByType(t *testing.T) {
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "Cat Tee", "6.1oz heavyweight", `{"price":"$18"}`, 0.9),
		hit(assets.AssetShippingPolicy, "Shipping SLA", "3-7 days production", "", 0.7),
		hit(assets.AssetCTA, "DM CTA", "Inbox mình nhé", "", 0.6),
		hit(assets.AssetPODProduct, "Cat Mug", "ceramic 11oz", "", 0.8),
	}, AssembleOptions{})

	// Each section header appears exactly once.
	for _, header := range []string{"PRODUCTS:", "POLICIES:", "CTAs:"} {
		if strings.Count(out, header) != 1 {
			t.Errorf("expected one occurrence of %q; got %d. Output:\n%s",
				header, strings.Count(out, header), out)
		}
	}
	// Products section orders by input order (caller pre-sorted by score).
	productsIdx := strings.Index(out, "PRODUCTS:")
	policiesIdx := strings.Index(out, "POLICIES:")
	if productsIdx < 0 || policiesIdx < 0 || productsIdx > policiesIdx {
		t.Error("PRODUCTS section should appear before POLICIES")
	}
	if !strings.Contains(out, "price: $18") {
		t.Error("product price should be surfaced from payload")
	}
}

func TestAssembleContext_DropsBannedClaims(t *testing.T) {
	// Banned claims must NEVER end up in the prompt — they belong to
	// a separate runtime guard.
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetBannedClaim, "best price guaranteed", "Cannot prove", "", 0.99),
		hit(assets.AssetPODProduct, "Cat Tee", "", "", 0.5),
	}, AssembleOptions{})
	if strings.Contains(out, "best price") {
		t.Errorf("banned claim leaked into assembled context:\n%s", out)
	}
	if !strings.Contains(out, "Cat Tee") {
		t.Error("non-banned product should survive")
	}
}

func TestAssembleContext_MaxProductsCaps(t *testing.T) {
	// Titles are distinct multi-character strings so substring-contain
	// asserts cannot collide with section headers like "PRODUCTS:".
	hits := []retrieval.Hit{
		hit(assets.AssetPODProduct, "alpha-tee", "", "", 0.9),
		hit(assets.AssetPODProduct, "bravo-mug", "", "", 0.8),
		hit(assets.AssetPODProduct, "charlie-hoodie", "", "", 0.7),
		hit(assets.AssetPODProduct, "delta-tote", "", "", 0.6),
	}
	out := AssembleContext(hits, AssembleOptions{MaxProducts: 2})
	for _, want := range []string{"alpha-tee", "bravo-mug"} {
		if !strings.Contains(out, want) {
			t.Errorf("top product %q missing from output", want)
		}
	}
	for _, notWant := range []string{"charlie-hoodie", "delta-tote"} {
		if strings.Contains(out, notWant) {
			t.Errorf("MaxProducts=2 should drop %q; output:\n%s", notWant, out)
		}
	}
}

func TestAssembleContext_EmptyFallback(t *testing.T) {
	got := AssembleContext(nil, AssembleOptions{EmptyFallback: "fallback text"})
	if got != "fallback text" {
		t.Errorf("empty input must return EmptyFallback verbatim; got %q", got)
	}
	got = AssembleContext(nil, AssembleOptions{})
	if got != "" {
		t.Errorf("empty input + empty fallback must be \"\"; got %q", got)
	}
}

func TestAssembleContext_MetricsAnnotation(t *testing.T) {
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "X", "", "", 0.87),
	}, AssembleOptions{IncludeMetrics: true})
	if !strings.Contains(out, "score=0.87") {
		t.Errorf("IncludeMetrics should annotate scores; got:\n%s", out)
	}
}
