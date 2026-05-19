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

// payloadV1ProductFixture is a literal PR-1 PayloadV1 JSON blob —
// the exact shape the rest_json adapter persists. Keeping it as raw
// JSON (rather than constructing through the products package) keeps
// the assembly test free of an extra runtime import and proves the
// renderer reads the on-disk contract, not the in-memory struct.
func payloadV1ProductFixture(opts struct {
	priceMin, priceMax string // empty = field absent
	currency           string
	origin             string
	sizes              []string
	sku                string
	sourceURL          string
	availability       string // "" defaults to "unknown"
}) string {
	parts := []string{`"schema_version":1`}
	parts = append(parts, `"source_id":"p1"`)
	parts = append(parts, `"name":"Stainless Travel Mug"`)
	parts = append(parts, `"extractor_version":"rest_json/v1"`)
	parts = append(parts, `"source_updated_at":"2026-05-17T15:44:47Z"`)
	if opts.priceMin != "" {
		parts = append(parts, `"price_min":`+opts.priceMin)
	}
	if opts.priceMax != "" {
		parts = append(parts, `"price_max":`+opts.priceMax)
	}
	if opts.currency != "" {
		parts = append(parts, `"currency":"`+opts.currency+`"`)
	}
	if opts.origin != "" {
		parts = append(parts, `"origin":"`+opts.origin+`"`)
	}
	if len(opts.sizes) > 0 {
		var quoted []string
		for _, s := range opts.sizes {
			quoted = append(quoted, `"`+s+`"`)
		}
		parts = append(parts, `"sizes":[`+strings.Join(quoted, ",")+`]`)
	}
	if opts.sku != "" {
		parts = append(parts, `"display_sku":"`+opts.sku+`"`)
	}
	if opts.sourceURL != "" {
		parts = append(parts, `"source_url":"`+opts.sourceURL+`"`)
	}
	if opts.availability != "" {
		parts = append(parts, `"availability":"`+opts.availability+`"`)
	} else {
		parts = append(parts, `"availability":"unknown"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestRenderProduct_PayloadV1_SurfacesPriceRangeOriginSKUSourceURL(t *testing.T) {
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "Stainless Travel Mug", "", payloadV1ProductFixture(struct {
			priceMin, priceMax string
			currency           string
			origin             string
			sizes              []string
			sku                string
			sourceURL          string
			availability       string
		}{
			priceMin:     "17.5",
			priceMax:     "22",
			currency:     "USD",
			origin:       "US",
			sizes:        []string{"S", "M", "L"},
			sku:          "THG-US002-RHLG5I",
			sourceURL:    "https://www.thgfulfill.com/catalog/p1",
			availability: "in_stock",
		}), 0.9),
	}, AssembleOptions{})

	wants := []string{
		"Stainless Travel Mug",
		"origin: US",
		"price: 17.5-22 USD",
		"sizes: S/M/L",
		"sku: THG-US002-RHLG5I",
		"https://www.thgfulfill.com/catalog/p1",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in output:\n%s", w, out)
		}
	}
}

func TestRenderProduct_PayloadV1_SinglePrice_NoCurrency(t *testing.T) {
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "Free product", "", payloadV1ProductFixture(struct {
			priceMin, priceMax string
			currency           string
			origin             string
			sizes              []string
			sku                string
			sourceURL          string
			availability       string
		}{priceMin: "0", priceMax: "0"}), 0.5),
	}, AssembleOptions{})

	// Same min == max collapses to single number; missing currency
	// renders without the trailing unit (we do not invent USD).
	if !strings.Contains(out, "price: 0") {
		t.Errorf("expected unit-less single price; got:\n%s", out)
	}
	if strings.Contains(out, "0-0") {
		t.Errorf("min==max should collapse, not render '0-0':\n%s", out)
	}
}

func TestRenderProduct_PayloadV1_OutOfStock_SurfacesWarning(t *testing.T) {
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "Discontinued tee", "", payloadV1ProductFixture(struct {
			priceMin, priceMax string
			currency           string
			origin             string
			sizes              []string
			sku                string
			sourceURL          string
			availability       string
		}{availability: "out_of_stock"}), 0.5),
	}, AssembleOptions{})
	if !strings.Contains(out, "out of stock") {
		t.Errorf("availability warning missing for out_of_stock; got:\n%s", out)
	}
}

func TestRenderProduct_PayloadV1_InStock_NoNoiseAnnotation(t *testing.T) {
	// in_stock is the default assumption — should NOT add an
	// "in stock" annotation that wastes prompt tokens.
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "Active tee", "", payloadV1ProductFixture(struct {
			priceMin, priceMax string
			currency           string
			origin             string
			sizes              []string
			sku                string
			sourceURL          string
			availability       string
		}{availability: "in_stock"}), 0.5),
	}, AssembleOptions{})
	if strings.Contains(out, "in stock") || strings.Contains(strings.ToLower(out), "availability") {
		t.Errorf("default in_stock should NOT surface annotation; got:\n%s", out)
	}
}

func TestRenderProduct_LegacyPayload_StillWorks(t *testing.T) {
	// Pre-PR-1 legacy payload — naive {"price":"$X"} string — must
	// still render correctly. The existing TestAssembleContext_GroupsByType
	// already covers the happy path; this one is explicit.
	out := AssembleContext([]retrieval.Hit{
		hit(assets.AssetPODProduct, "Legacy item", "6.1oz heavyweight", `{"price":"$18"}`, 0.5),
	}, AssembleOptions{})
	if !strings.Contains(out, "price: $18") {
		t.Errorf("legacy payload extraction broke; output:\n%s", out)
	}
	if !strings.Contains(out, "6.1oz heavyweight") {
		t.Errorf("legacy description missing; output:\n%s", out)
	}
}
