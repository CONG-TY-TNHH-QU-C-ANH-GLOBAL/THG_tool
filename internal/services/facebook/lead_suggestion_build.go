package facebook

import (
	"context"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/models"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// BuildLeadSuggestion performs the optional, operator-facing suggestion work.
// It is intentionally best-effort: retrieval or generation failure may omit
// a draft and must never affect lead ingestion.
func BuildLeadSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, msgGen *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, leadText, author string, shippingQuote ShippingQuoteFunc, supplierLookup SupplierLookupFunc) LeadSuggestion {
	var candidates []models.KnowledgeCandidate
	if builder != nil {
		var err error
		candidates, _, err = builder.CandidatesForLead(ctx, orgID, leadText)
		if err != nil {
			// Live sourcing can still resolve a product when the local knowledge
			// retrieval is temporarily unavailable.
			candidates = nil
		}
	}
	matched := matchingCandidates(leadText, candidates)
	product := PickSuggestedProductDetails(matched)
	var supplier *models.SupplierMatch
	lookupStatus := classifySupplierLookupError(nil, supplierLookup == nil)
	if wantsBulkSourcing(leadText) && !wantsPersonalizedPOD(leadText) {
		product = SuggestedProduct{}
	}
	if product.URL == "" {
		supplier = PickSuggestedSupplier(matched)
		if linkedURL, _ := leadMarketplaceURL(leadText); linkedURL != "" && supplier != nil {
			// An exact seller link in the post outranks a similarly titled item
			// from the approved index; the indexed copy of that link is exact.
			if !sameMarketplaceListing(supplier.URL, linkedURL) {
				supplier = nil
			} else {
				supplier.Similar = false
			}
		}
		if !freshSupplierPrice(supplier, time.Now()) {
			supplier = nil
		}
		var liveWeight *float64
		if !supplier.HasOffer() && supplierLookup != nil {
			resolved, lookupErr := supplierLookup(ctx, leadText)
			lookupStatus = classifySupplierLookupError(lookupErr, false)
			if lookupErr == nil && resolved != nil {
				supplier, liveWeight = resolved.Match, resolved.WeightKG
			}
		}
		// CRM may return a one-parcel reference for a bulk post, labelled as
		// such. It never treats that figure as the shipment total.
		if supplier.HasOffer() && shippingQuote != nil && leadDestinationCountry(leadText) != "" {
			quantity := leadQuantity(leadText)
			mode := "parcel"
			if wantsBulkSourcing(leadText) && quantity > 1 {
				mode = "bulk"
			}
			cargo := "unknown"
			if isOrdinaryApparel(supplier.Name) {
				cargo = "standard"
			}
			quoteForWeight := func(weight float64) {
				if quantity > 0 && (mode == "bulk" || quantity == 1) && weight > 0 && weight <= 20 {
					supplier.Shipping, _ = shippingQuote(ctx, models.ShippingRequest{
						OriginCountry: "CN", DestinationCountry: leadDestinationCountry(leadText),
						Quantity: quantity, ShipmentMode: mode, CargoCategory: cargo, WeightKG: weight,
					})
				}
			}
			if liveWeight != nil && *liveWeight > 0 && *liveWeight <= 20 {
				quoteForWeight(*liveWeight)
			} else {
				for _, candidate := range matched {
					if candidate.SourceURL == supplier.URL && candidate.Supplier != nil && candidate.Supplier.WeightKG != nil {
						quoteForWeight(*candidate.Supplier.WeightKG)
						break
					}
				}
			}
		}
	}
	if product.Name == "" && product.URL == "" && !supplier.HasOffer() {
		return noOfferSuggestionForLookup(leadText, author, lookupStatus)
	}
	if supplier.HasOffer() && wantsBulkSourcing(leadText) && leadQuantity(leadText) > 1 &&
		!strings.Contains(supplier.PriceText, "giá bậc") && !strings.Contains(supplier.PriceText, "tier ") &&
		!strings.Contains(supplier.PriceText, "MOQ") {
		if supplierQueryLanguage(leadText) == "en" {
			supplier.PriceText += " (volume price to confirm)"
		} else {
			supplier.PriceText += " (giá theo số lượng cần xác nhận)"
		}
	}
	out := LeadSuggestion{
		ProductName: product.Name, ProductURL: product.URL, ProductImageURL: product.ImageURL,
		Supplier: supplier,
	}
	if supplier.HasOffer() {
		// Sourcing facts already provide a concise, copy-ready operator draft.
		// Assemble it deterministically so an LLM cannot change the price or
		// turn a one-parcel reference into a quote for the whole shipment.
		out.Reply = supplierReplyForIntent(author, supplier, leadText) + " " + supplier.URL
		return out
	}
	if msgGen == nil || !msgGen.Available() || profile == nil {
		out.Reply = podFallbackReply(author, product, leadText)
		return out
	}
	reply, err := msgGen.GenerateLeadReplySuggestion(ctx, ai.LeadReplyRequest{
		PostContent:     leadText,
		AuthorName:      author,
		BusinessContext: profile.ToPromptBlock(),
		BrandName:       profile.Name,
		GroundedFacts:   BuildGroundedFacts(product, supplier),
	})
	if err != nil {
		out.Reply = podFallbackReply(author, product, leadText)
		return out
	}
	out.Reply = strings.TrimSpace(reply)
	if out.Reply == "" || !generatedPODReplyGrounded(out.Reply, product.URL) {
		out.Reply = podFallbackReply(author, product, leadText)
		return out
	}
	selectedURL := product.URL
	if product.URL != "" && len([]rune(out.Reply)) > 320 {
		out.Reply = podFallbackReply(author, product, leadText)
	}
	if selectedURL != "" && !strings.Contains(out.Reply, selectedURL) {
		out.Reply = strings.TrimSpace(out.Reply + " " + selectedURL)
	}
	return out
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
