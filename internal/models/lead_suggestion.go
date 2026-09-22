package models

// LeadSuggestion is optional operator-facing enrichment for a new-lead
// notification. URLs are copied from persisted catalog assets, never generated.
type LeadSuggestion struct {
	Reply           string
	ProductName     string
	ProductURL      string
	ProductImageURL string
	// ShippingLine is a cost the CRM computed from a published rate card, shown
	// on its own line in the Telegram notice. Empty when the lead named no
	// marketplace product, or the product published no weight.
	ShippingLine string
}
