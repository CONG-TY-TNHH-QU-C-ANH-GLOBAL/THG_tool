package facebook

import (
	"context"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// buildGroundedLeadSuggestion selects facts independently of model wording.
func buildGroundedLeadSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, orgID int64, leadText, author string, shippingQuote ShippingQuoteFunc, supplierLookup SupplierLookupFunc) LeadSuggestion {
	if early, ok := earlyLeadSuggestion(leadText, author); ok {
		return early
	}
	candidates, _, err := builder.ProductCandidatesForLead(ctx, orgID, leadText)
	if err != nil {
		candidates = nil // Live lookup may still resolve the product.
	}
	matched := matchingCandidates(leadText, candidates)
	product := PickSuggestedProductDetails(matched)
	if (wantsBulkSourcing(leadText) || asksForSupplier(leadText)) && !wantsPersonalizedPOD(leadText) {
		product = SuggestedProduct{}
	}
	var supplier *models.SupplierMatch
	status := classifySupplierLookupError(nil, supplierLookup == nil)
	if product.URL == "" {
		var weight *float64
		supplier, weight, status = resolveLeadSupplier(ctx, leadText, matched, supplierLookup)
		quoteLeadSupplier(ctx, leadText, matched, supplier, weight, shippingQuote)
	}
	if product.Name == "" && product.URL == "" && !supplier.HasOffer() {
		return noOfferSuggestionForLookup(leadText, author, status)
	}
	qualifyVolumePrice(supplier, leadText)
	out := LeadSuggestion{ProductName: product.Name, ProductURL: product.URL, ProductImageURL: product.ImageURL, Supplier: supplier}
	if supplier.HasOffer() {
		out.Reply = supplierReplyForIntent(author, supplier, leadText) + " " + supplier.URL
	} else {
		out.Reply = podFallbackReply(author, product, leadText)
	}
	return out
}

func earlyLeadSuggestion(text, author string) (LeadSuggestion, bool) {
	if containsAnyLeadPhrase(text, []string{"looking for a virtual assistant", "need a virtual assistant", "hiring a virtual assistant", "tuyển trợ lý ảo"}) {
		return LeadSuggestion{}, true
	}
	link, _ := leadMarketplaceURL(text)
	if link != "" || supplierQuery(text) != "" || wantsPersonalizedPOD(text) {
		return LeadSuggestion{}, false
	}
	if service := serviceInquirySuggestion(text, author); service.Reply != "" {
		return service, true
	}
	if asksForSupplier(text) {
		return noOfferSuggestionForLookup(text, author, lookupCompleted), true
	}
	return LeadSuggestion{}, false
}

func qualifyVolumePrice(supplier *models.SupplierMatch, text string) {
	if !supplier.HasOffer() || !wantsBulkSourcing(text) || leadQuantity(text) <= 1 || strings.TrimSpace(supplier.PriceText) == "" {
		return
	}
	if strings.Contains(supplier.PriceText, "giá bậc") || strings.Contains(supplier.PriceText, "tier ") || strings.Contains(supplier.PriceText, "MOQ") {
		return
	}
	if supplierQueryLanguage(text) == "en" {
		supplier.PriceText += " (volume price to confirm)"
	} else {
		supplier.PriceText += " (giá theo số lượng cần xác nhận)"
	}
}

func freshSupplierPrice(supplier *models.SupplierMatch, now time.Time) bool {
	if !supplier.HasOffer() || supplier.PriceText == "" {
		return false
	}
	captured, err := time.Parse(time.RFC3339, supplier.CapturedAt)
	if err != nil {
		return false
	}
	age := now.Sub(captured)
	return age >= 0 && age <= 7*24*time.Hour
}
