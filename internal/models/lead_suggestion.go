package models

// LeadSuggestion is optional operator-facing enrichment for a new-lead
// notification. URLs come from catalog assets or verified Pricing Hub results.
type LeadSuggestion struct {
	Reply           string
	ProductName     string
	ProductURL      string
	ProductImageURL string
	// Supplier is one matched 1688/Taobao offer from the approved index or
	// Pricing Hub. The company catalog item takes priority for a POD lead.
	Supplier *SupplierMatch
	// SourcingNote distinguishes no match, insufficient details and an unfinished
	// lookup so an ask-for-details draft is not mistaken for a sourced offer.
	SourcingNote string
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
	Shipping   *ShippingReference
	// Similar marks an offer matched by title rather than a link from the post.
	// Without image matching it may not be the exact model the lead asked for.
	Similar bool
}

// ShippingReference is a published per-parcel estimate from CRM's rate card.
// It is never a quote for the customer's full monthly volume.
type ShippingReference struct {
	PriceText string
	Transit   string
	Basis     string
	SourceURL string
}

// ShippingRequest carries the post's explicit volume and destination into the
// pricing router. Quantity zero means the post did not state one.
type ShippingRequest struct {
	OriginCountry, DestinationCountry string
	Quantity                          int
	ShipmentMode, CargoCategory       string
	WeightKG                          float64
}

// HasOffer reports whether the match carries enough to be worth showing: a name
// and a link. Numbers alone are not an offer.
func (s *SupplierMatch) HasOffer() bool {
	return s != nil && s.Name != "" && s.URL != ""
}
