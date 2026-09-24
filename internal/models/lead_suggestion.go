package models

// LeadSuggestion is optional operator-facing enrichment for a new-lead
// notification. URLs are copied from persisted catalog assets, never generated.
type LeadSuggestion struct {
	Reply           string
	ProductName     string
	ProductURL      string
	ProductImageURL string

	// ProductLine is the one-line product summary the notice shows:
	// "<tên> · 20 CNY · MOQ 2包". Built from the CRM lookup, so it carries the
	// real marketplace price — ProductName alone never did.
	ProductLine string

	// ShippingLine is the cost the CRM computed from a published rate card
	// ("$14.20 · 6–12 ngày làm việc"), and ShippingBasis is what it was computed
	// from ("Epacket CN→US, 1 kiện 0.4 kg").
	//
	// They are separate fields, not one string, because the notice puts the
	// basis on its own indented line under the number. A landed cost with no
	// stated basis is a figure nobody can check before quoting it to a customer.
	ShippingLine  string
	ShippingBasis string
}
