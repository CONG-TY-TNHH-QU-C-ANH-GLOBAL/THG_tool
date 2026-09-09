package facebook

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// Sourcing enrichment for the operator lead notice. PURE: the caller performs
// retrieval and hands the ranked candidates here; this file only selects and
// formats. Nothing is computed that the marketplace did not publish — a missing
// weight stays missing.

// PickSuggestedSupplier returns the best-ranked sourceable marketplace item from
// an already-ranked candidate list, or nil when none carries a usable link.
// Retrieval owns the ranking, so the first eligible candidate is the match.
func PickSuggestedSupplier(candidates []models.KnowledgeCandidate) *models.SupplierMatch {
	for _, c := range candidates {
		if c.Kind != string(assets.AssetSupplierProduct) || c.Supplier == nil {
			continue
		}
		link := validHTTPSURL(c.SourceURL)
		if link == "" {
			continue
		}
		name := strings.TrimSpace(c.Title)
		if name == "" {
			continue
		}
		return &models.SupplierMatch{
			Platform:   c.Supplier.PlatformLabel(),
			Name:       name,
			URL:        link,
			ImageURL:   validHTTPSURL(c.ImageURL),
			PriceText:  formatYuan(c.Supplier.PriceCNY),
			WeightKG:   formatWeight(c.Supplier.WeightKG),
			MOQText:    formatMOQ(c.Supplier.MOQ, c.Supplier.Unit),
			ShipFrom:   strings.TrimSpace(c.Supplier.ShipFrom),
			ShopName:   strings.TrimSpace(c.Supplier.ShopName),
			CapturedAt: formatCapturedAt(c.Supplier.CapturedAt),
		}
	}
	return nil
}

// BuildGroundedFacts renders the facts block the reply generator may quote from.
// It is the ONLY channel through which product numbers reach the model, so a
// number absent here can never appear in the reply.
func BuildGroundedFacts(product SuggestedProduct, supplier *models.SupplierMatch) string {
	var b strings.Builder
	if product.Name != "" {
		b.WriteString("OUR CATALOG PRODUCT:\n")
		writeFact(&b, "name", product.Name)
		writeFact(&b, "link", product.URL)
	}
	if supplier.HasOffer() {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("SOURCEABLE ITEM (we can source and fulfil this for the customer):\n")
		writeFact(&b, "marketplace", supplier.Platform)
		writeFact(&b, "name", supplier.Name)
		writeFact(&b, "unit price", supplier.PriceText)
		writeFact(&b, "shipping weight", supplier.WeightKG)
		writeFact(&b, "minimum order", supplier.MOQText)
		writeFact(&b, "ships from", supplier.ShipFrom)
		writeFact(&b, "link", supplier.URL)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeFact(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", label, value)
}

// formatYuan renders a marketplace price. The currency symbol is kept because
// the operator must see that the number is not already in their own currency.
func formatYuan(price *float64) string {
	if price == nil || *price <= 0 {
		return ""
	}
	return "¥" + strconv.FormatFloat(*price, 'f', -1, 64)
}

func formatWeight(weight *float64) string {
	if weight == nil || *weight <= 0 {
		return ""
	}
	return strconv.FormatFloat(*weight, 'f', -1, 64) + " kg"
}

// formatMOQ renders the minimum order with the marketplace's own unit word
// ("件", "个"), which is what the supplier page shows.
func formatMOQ(moq *int, unit string) string {
	if moq == nil || *moq <= 0 {
		return ""
	}
	out := strconv.Itoa(*moq)
	if unit = strings.TrimSpace(unit); unit != "" {
		out += " " + unit
	}
	return out
}

func formatCapturedAt(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}
