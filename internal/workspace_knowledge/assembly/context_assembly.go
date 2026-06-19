// Package assembly is Layer 5 of the Workspace Knowledge OS: it
// turns a list of [retrieval.Hit]s into a structured prompt block
// the agent runtime can paste into the system prompt.
//
// Why this lives in its own package: the agent runtime (Layer 6)
// must not know about scoring, retrieval, or storage. It accepts a
// finished string. If the team later inserts a re-ranking step or a
// per-type formatting tweak, this is the only file that changes.
//
// Output is plain text — not JSON — because the receiving LLM prompt
// is plain text. We tag each section with a stable token
// ("PRODUCTS:", "POLICIES:", "CTAs:") so a future caller can locate
// and surgically replace one section without re-running the search.
package assembly

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/products"
	"github.com/thg/scraper/internal/workspace_knowledge/retrieval"
)

// AssembleOptions controls how the prompt block is formatted.
type AssembleOptions struct {
	// MaxProducts caps the product list in the assembled block. 0
	// means "no cap." Comments in Vietnamese typically perform best
	// with 1-2 products; inbox messages can handle 3-4.
	MaxProducts int

	// IncludeMetrics decides whether to surface "retrieved 184 times,
	// 27 conversions" annotations. Useful in operator-facing surfaces;
	// noise in LLM-facing prompts. Default false.
	IncludeMetrics bool

	// EmptyFallback is the string returned when there are no hits.
	// The runtime treats an empty string as "no catalog context
	// available" and falls back to the legacy freeform business
	// profile.
	EmptyFallback string
}

// AssembleContext renders hits into a prompt block. Asset types are
// grouped (POD products → "PRODUCTS:", shipping/pricing/playbooks →
// "POLICIES:", CTAs → "CTAs:") so the LLM can reason about each
// section separately. Banned claims are NOT included here — they go
// through a separate runtime guard, not through prompt injection.
//
// Returns "" when len(hits) == 0 and EmptyFallback is empty.
//
// For the explainability-aware path, callers should use
// [AssembleWithBudget] which returns the same string plus a
// [retrieval.AssemblyBudget] describing what was trimmed and why.
// AssembleContext is the convenience wrapper for callers that do
// not need the trace.
func AssembleContext(hits []retrieval.Hit, opts AssembleOptions) string {
	out, _ := AssembleWithBudget(hits, opts)
	return out
}

// AssembleWithBudget is the explainability variant. The second return
// value carries the budget-trimming detail the Operator Replay UI
// uses to show "we assembled 2 products and dropped 1 by MaxProducts
// cap, plus 1 banned claim was filtered."
func AssembleWithBudget(hits []retrieval.Hit, opts AssembleOptions) (string, retrieval.AssemblyBudget) {
	budget := retrieval.AssemblyBudget{}
	if len(hits) == 0 {
		return opts.EmptyFallback, budget
	}
	products, policies, ctas, complianceDropped := splitByType(hits)
	budget.ComplianceDropped = complianceDropped

	if opts.MaxProducts > 0 && len(products) > opts.MaxProducts {
		budget.DroppedByCap = len(products) - opts.MaxProducts
		products = products[:opts.MaxProducts]
	}
	budget.AssembledProducts = len(products)
	budget.AssembledPolicies = len(policies)
	budget.AssembledCTAs = len(ctas)

	var b strings.Builder
	writeSection(&b, "PRODUCTS", products, opts.IncludeMetrics, renderProduct)
	writeSection(&b, "POLICIES", policies, opts.IncludeMetrics, renderPolicy)
	writeSection(&b, "CTAs", ctas, opts.IncludeMetrics, renderCTA)
	out := strings.TrimSpace(b.String())
	budget.EstimatedTokens = estimateTokens(out)
	return out, budget
}

// estimateTokens is a deliberately crude approximation: characters /
// 4. The replay UI uses this for a "this prompt block costs ~N tokens"
// label, not for billing — exact token count requires the model's
// tokenizer which we do not embed.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// splitByType groups hits by asset type into the three sections the
// LLM prompt cares about. Banned claims are dropped (they never belong
// in a generation prompt; the runtime guard handles them post-generation)
// — the dropped count is returned so the trace can record exactly how
// many compliance drops happened on this retrieval.
func splitByType(hits []retrieval.Hit) (products, policies, ctas []retrieval.Hit, complianceDropped int) {
	for _, h := range hits {
		if h.Asset == nil {
			continue
		}
		switch h.Asset.Type {
		case assets.AssetPODProduct:
			products = append(products, h)
		case assets.AssetShippingPolicy, assets.AssetPricingRule, assets.AssetSalesPlaybook, assets.AssetFAQ:
			policies = append(policies, h)
		case assets.AssetCTA:
			ctas = append(ctas, h)
		case assets.AssetBannedClaim:
			complianceDropped++
		}
	}
	return
}

func writeSection(b *strings.Builder, label string, hits []retrieval.Hit, withMetrics bool, render func(retrieval.Hit, bool) string) {
	if len(hits) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, h := range hits {
		fmt.Fprintf(b, "  - %s\n", render(h, withMetrics))
	}
	b.WriteString("\n")
}

func renderProduct(h retrieval.Hit, withMetrics bool) string {
	a := h.Asset
	parts := []string{a.Title}

	// Try the structured PayloadV1 first (PR-1 product-catalog assets).
	// Every product written by the rest_json / shopify / future
	// product-catalog adapters carries this shape; an unmarshal that
	// returns SchemaVersion >= 1 means the payload is one of ours.
	//
	// Falling back to the legacy {"price":"$X"} substring extraction
	// below keeps the seven existing test fixtures green and tolerates
	// any pre-Workspace-Knowledge POD assets that may still sit in the
	// org's catalog table.
	var pv products.PayloadV1
	if len(a.Payload) > 0 && json.Unmarshal(a.Payload, &pv) == nil && pv.SchemaVersion > 0 {
		return strings.Join(appendStructuredProductParts(parts, pv, h.Score, withMetrics), " · ")
	}

	// Legacy path: free-form Description + naive substring price.
	if a.Description != "" {
		parts = append(parts, truncate(a.Description, 160))
	}
	if price := payloadField(a, "price"); price != "" {
		parts = append(parts, "price: "+price)
	}
	if withMetrics {
		parts = append(parts, fmt.Sprintf("score=%.2f", h.Score))
	}
	return strings.Join(parts, " · ")
}

// appendStructuredProductParts appends the structured PayloadV1 fields (origin,
// price, sizes, sku, availability constraint, source URL, optional score) onto
// parts in the exact order renderProduct's structured branch produced them.
// Extracted verbatim from renderProduct so the renderer stays flat.
func appendStructuredProductParts(parts []string, pv products.PayloadV1, score float64, withMetrics bool) []string {
	if pv.Origin != "" {
		parts = append(parts, "origin: "+pv.Origin)
	}
	if priceStr := formatPriceRange(pv.PriceMin, pv.PriceMax, pv.Currency); priceStr != "" {
		parts = append(parts, "price: "+priceStr)
	}
	if len(pv.Sizes) > 0 {
		parts = append(parts, "sizes: "+strings.Join(pv.Sizes, "/"))
	}
	if pv.DisplaySKU != "" {
		parts = append(parts, "sku: "+pv.DisplaySKU)
	}
	// Surface availability ONLY when it signals a constraint the
	// LLM should reflect in its draft. in_stock is the default
	// assumption; unknown is noise; the other three values matter.
	switch pv.Availability {
	case products.AvailLowStock:
		parts = append(parts, "low stock")
	case products.AvailOutOfStock:
		parts = append(parts, "out of stock")
	case products.AvailDiscontinued:
		parts = append(parts, "discontinued")
	}
	// Source URL lands LAST so the LLM can choose to drop it into
	// the comment as a citation (or omit it for casual tone).
	if pv.SourceURL != "" {
		parts = append(parts, pv.SourceURL)
	}
	if withMetrics {
		parts = append(parts, fmt.Sprintf("score=%.2f", score))
	}
	return parts
}

// formatPriceRange renders the canonical price interval into the
// short format the prompt block uses. Examples:
//
//	min=17.5  max=22    currency=USD  → "17.5-22 USD"
//	min=22    max=22    currency=USD  → "22 USD"
//	min=22    max=nil   currency=""   → "22"
//	min=nil   max=nil                 → ""           (no price block)
//
// Currency is appended only when non-empty; some upstream feeds
// expose prices without a unit and we surface them as-is rather than
// inventing a currency.
func formatPriceRange(minPrice, maxPrice *float64, currency string) string {
	if minPrice == nil && maxPrice == nil {
		return ""
	}
	a := minPrice
	b := maxPrice
	if a == nil {
		a = b
	}
	if b == nil {
		b = a
	}
	currency = strings.TrimSpace(currency)
	left := strconv.FormatFloat(*a, 'f', -1, 64)
	if *a == *b {
		if currency == "" {
			return left
		}
		return left + " " + currency
	}
	right := strconv.FormatFloat(*b, 'f', -1, 64)
	out := left + "-" + right
	if currency != "" {
		out += " " + currency
	}
	return out
}

func renderPolicy(h retrieval.Hit, withMetrics bool) string {
	a := h.Asset
	out := a.Title
	if a.Description != "" {
		out += ": " + truncate(a.Description, 220)
	}
	if withMetrics {
		out += fmt.Sprintf(" (score=%.2f)", h.Score)
	}
	return out
}

func renderCTA(h retrieval.Hit, withMetrics bool) string {
	a := h.Asset
	// CTAs are usually one-liners and the Description IS the text the
	// AI should mimic. Surface it verbatim.
	text := a.Description
	if text == "" {
		text = a.Title
	}
	if withMetrics {
		text += fmt.Sprintf(" [score=%.2f]", h.Score)
	}
	return text
}

func payloadField(a *assets.Asset, key string) string {
	// We avoid full JSON parsing in the hot path. The payload is
	// constructed by ingestors using map[string]string-style JSON; a
	// naive substring lookup gets the price field out without an
	// unmarshal. Production-grade would use json.RawMessage + lazy
	// decode; not worth the allocation here for one field.
	needle := `"` + key + `":"`
	_, rest, ok := strings.Cut(string(a.Payload), needle)
	if !ok {
		return ""
	}
	value, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return ""
	}
	return value
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
