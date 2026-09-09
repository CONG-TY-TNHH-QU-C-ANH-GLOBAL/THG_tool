package models

// LeadSuggestion is optional operator-facing enrichment for a new-lead
// notification. URLs are copied from persisted catalog assets, never generated.
type LeadSuggestion struct {
	Reply           string
	ProductName     string
	ProductURL      string
	ProductImageURL string
	// Supplier is the matched sourceable marketplace item (1688 / Taobao), when
	// one was indexed for this workspace. It is a SECOND, distinct offer from
	// the catalog product above: the catalog link is what the business sells,
	// the supplier link is what it can source. nil when nothing matched.
	Supplier *SupplierMatch
}

// SupplierMatch is the render-ready view of one sourced marketplace item. Every
// field is already formatted for display and every one may be empty — the sink
// omits what is missing rather than substituting a default.
type SupplierMatch struct {
	Platform   string // "1688" | "Taobao"
	Name       string
	URL        string
	ImageURL   string
	PriceText  string // e.g. "¥28.9"
	WeightKG   string // e.g. "0.29 kg"
	MOQText    string // e.g. "1 件"
	ShipFrom   string
	ShopName   string
	CapturedAt string // when the price was read upstream, RFC3339; may be empty
}

// HasOffer reports whether the match carries enough to be worth showing: a name
// and a link. Numbers alone are not an offer.
func (s *SupplierMatch) HasOffer() bool {
	return s != nil && s.Name != "" && s.URL != ""
}
