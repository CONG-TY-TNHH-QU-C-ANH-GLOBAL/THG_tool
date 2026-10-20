package facebook

import (
	"context"
	"time"

	"github.com/thg/scraper/internal/models"
)

func resolveLeadSupplier(ctx context.Context, text string, matched []models.KnowledgeCandidate, lookup SupplierLookupFunc) (*models.SupplierMatch, *float64, supplierLookupStatus) {
	supplier := indexedLeadSupplier(text, matched)
	status := classifySupplierLookupError(nil, lookup == nil)
	linkedURL, _ := leadMarketplaceURL(text)
	if supplier.HasOffer() || lookup == nil || (supplierQuery(text) == "" && linkedURL == "") {
		return supplier, nil, status
	}
	resolved, err := lookup(ctx, text)
	status = classifySupplierLookupError(err, false)
	if err == nil && resolved != nil {
		if resolved.Match.HasOffer() && !marketplaceProductURL(resolved.Match.URL) {
			return nil, nil, lookupUnavailable
		}
		return resolved.Match, resolved.WeightKG, status
	}
	return supplier, nil, status
}

func indexedLeadSupplier(text string, matched []models.KnowledgeCandidate) *models.SupplierMatch {
	supplier := PickSuggestedSupplier(matched)
	if linkedURL, _ := leadMarketplaceURL(text); linkedURL != "" && supplier != nil {
		if !sameMarketplaceListing(supplier.URL, linkedURL) {
			return nil
		}
		supplier.Similar = false
	}
	if !freshSupplierPrice(supplier, time.Now()) {
		return nil
	}
	return supplier
}

// Only a known destination, quantity and usable weight can reach the rate
// router. Monthly demand is never represented as a confirmed shipment total.
func quoteLeadSupplier(ctx context.Context, text string, matched []models.KnowledgeCandidate, supplier *models.SupplierMatch, liveWeight *float64, quote ShippingQuoteFunc) {
	if !supplier.HasOffer() || quote == nil || leadDestinationCountry(text) == "" {
		return
	}
	quantity := leadQuantity(text)
	mode := "parcel"
	if wantsBulkSourcing(text) && quantity > 1 {
		mode = "bulk"
	}
	weight := leadSupplierWeight(supplier, liveWeight, matched)
	if quantity <= 0 || (mode != "bulk" && quantity != 1) || weight <= 0 || weight > 20 {
		return
	}
	cargo := "unknown"
	if isOrdinaryApparel(supplier.Name) {
		cargo = "standard"
	}
	supplier.Shipping, _ = quote(ctx, models.ShippingRequest{
		OriginCountry: "CN", DestinationCountry: leadDestinationCountry(text),
		Quantity: quantity, ShipmentMode: mode, CargoCategory: cargo, WeightKG: weight,
	})
}

func leadSupplierWeight(supplier *models.SupplierMatch, liveWeight *float64, matched []models.KnowledgeCandidate) float64 {
	if liveWeight != nil && *liveWeight > 0 && *liveWeight <= 20 {
		return *liveWeight
	}
	for _, candidate := range matched {
		if candidate.SourceURL == supplier.URL && candidate.Supplier != nil && candidate.Supplier.WeightKG != nil {
			return *candidate.Supplier.WeightKG
		}
	}
	return 0
}
