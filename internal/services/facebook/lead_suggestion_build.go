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
	if wantsBulkSourcing(leadText) && !wantsPersonalizedPOD(leadText) {
		product = SuggestedProduct{}
	}
	if product.URL == "" {
		supplier = PickSuggestedSupplier(matched)
		if linkedURL, _ := leadMarketplaceURL(leadText); linkedURL != "" && supplier != nil && supplier.URL != linkedURL {
			// An exact seller link in the post outranks a similarly titled item
			// from the approved index.
			supplier = nil
		}
		if !freshSupplierPrice(supplier, time.Now()) {
			supplier = nil
		}
		var liveWeight *float64
		if !supplier.HasOffer() && supplierLookup != nil {
			resolved, lookupErr := supplierLookup(ctx, leadText)
			if lookupErr == nil && resolved != nil {
				supplier, liveWeight = resolved.Match, resolved.WeightKG
			}
		}
		// CRM may return a one-parcel reference for a bulk post, labelled as
		// such. It never treats that figure as the shipment total.
		if supplier.HasOffer() && shippingQuote != nil && leadDestinationCountry(leadText) != "" {
			quantity := leadQuantity(leadText)
			mode := "parcel"
			if wantsBulkSourcing(leadText) {
				mode = "bulk"
			}
			cargo := "unknown"
			if isOrdinaryApparel(supplier.Name) {
				cargo = "standard"
			}
			quoteForWeight := func(weight float64) {
				if quantity > 0 && weight > 0 && weight <= 20 {
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
	// The company's matching POD item takes priority. Marketplace sourcing is the
	// fallback, never a second unrelated offer in the same lead suggestion.
	if product.Name == "" && product.URL == "" && !supplier.HasOffer() {
		return LeadSuggestion{}
	}
	out := LeadSuggestion{
		ProductName: product.Name, ProductURL: product.URL, ProductImageURL: product.ImageURL,
		Supplier: supplier,
	}
	if supplier.HasOffer() {
		// Sourcing facts already provide a concise, copy-ready operator draft.
		// Assemble it deterministically so an LLM cannot change the price or
		// turn a one-parcel reference into a quote for the whole shipment.
		out.Reply = supplierFallbackReply(author, supplier, leadText) + " " + supplier.URL
		return out
	}
	if msgGen == nil || !msgGen.Available() || profile == nil {
		out.Reply = leadSalutation(author) + ", bên mình có " + shortLeadTitle(product.Name) + " phù hợp nhu cầu của bạn. Mình gửi chi tiết qua inbox nhé? " + product.URL
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
		return out
	}
	out.Reply = strings.TrimSpace(reply)
	selectedURL := product.URL
	if product.URL != "" && len([]rune(out.Reply)) > 320 {
		out.Reply = leadSalutation(author) + ", bên mình có " + shortLeadTitle(product.Name) + " phù hợp nhu cầu của bạn. Mình gửi thêm chi tiết qua inbox nhé?"
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

func supplierFallbackReply(author string, supplier *models.SupplierMatch, leadText string) string {
	if supplierEnglishQuery(leadText) != "" {
		first := "Hi " + leadSalutation(author) + ", we found a similar " + shortLeadTitle(supplier.Name) + " from China"
		if supplier.PriceText != "" {
			first += " at a reference product price of " + supplier.PriceText
		}
		if supplier.Shipping != nil && supplier.Shipping.PriceText != "" {
			first += "; reference shipping " + supplier.Shipping.PriceText + " per parcel"
			if strings.Contains(supplier.Shipping.Basis, "không phải tổng cước lô") {
				first += " (not the total bulk shipping cost)"
			}
		}
		missingFacts := "quantity and destination"
		if leadDestinationCountry(leadText) != "" {
			missingFacts = "quantity and parcel details"
		}
		if strings.Contains(strings.ToLower(leadText), "european") || strings.Contains(strings.ToLower(leadText), "supplier in europe") {
			return first + ". Would a China-based alternative work? Please share the " + missingFacts + " for a shipping quote."
		}
		return first + ". Please share the " + missingFacts + " for a shipping quote."
	}
	first := leadSalutation(author) + ", bên mình có thể tìm nguồn " + shortLeadTitle(supplier.Name)
	if supplier.PriceText != "" {
		first += ", giá nguồn tham khảo " + supplier.PriceText
	}
	if supplier.Shipping != nil && supplier.Shipping.PriceText != "" {
		first += "; cước tham chiếu " + supplier.Shipping.PriceText + "/kiện"
		if strings.Contains(supplier.Shipping.Basis, "không phải tổng cước lô") {
			first += " (chưa phải tổng cước lô)"
		}
		if supplier.Shipping.Transit != "" {
			first += " (" + supplier.Shipping.Transit + ")"
		}
	} else if destination := leadDestinationCountry(leadText); destination != "" {
		first += "; tuyến CN→" + destination + ", cước cần xác nhận theo cách giao và quy cách kiện"
	}
	return first + ". Mình trao đổi số lượng và báo giá cụ thể qua inbox nhé?"
}

func leadSalutation(author string) string {
	if name := strings.TrimSpace(author); name != "" {
		return name
	}
	return "Bạn"
}

func shortLeadTitle(title string) string {
	runes := []rune(strings.TrimSpace(title))
	if len(runes) > 100 {
		return string(runes[:100]) + "…"
	}
	return string(runes)
}
