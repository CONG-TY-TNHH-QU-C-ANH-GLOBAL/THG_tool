package products

// Availability is the closed set of stock states a product can be in.
// Adapters MUST map their upstream availability vocabulary into one of
// these values; downstream retrieval governance filters on this enum
// (a "discontinued" product never grounds an outbound comment, even
// if it was once retrieved successfully).
type Availability string

const (
	// AvailUnknown is the safe default when the upstream system did
	// not expose an availability signal. Retrieval may still pick the
	// product, but the agent prompt should hedge ("availability not
	// confirmed").
	AvailUnknown Availability = "unknown"

	AvailInStock      Availability = "in_stock"
	AvailLowStock     Availability = "low_stock"
	AvailOutOfStock   Availability = "out_of_stock"
	AvailDiscontinued Availability = "discontinued"
)

// IsKnown reports whether a is one of the supported availability
// values. Adapters call this when mapping upstream values so unknown
// upstream states surface as a validation error instead of silently
// becoming AvailUnknown — silent coercion masks real schema drift.
func (a Availability) IsKnown() bool {
	switch a {
	case AvailUnknown, AvailInStock, AvailLowStock, AvailOutOfStock, AvailDiscontinued:
		return true
	}
	return false
}

// ProductVariant is one purchasable configuration of a parent product
// (size M / colour red / fabric 250gsm …). The list-level Sizes/Colours
// slices on CanonicalProduct are derived sets; Variants is where the
// (price, image, availability) per combination lives.
//
// Adapters that have no concept of variants leave the slice empty.
// Adapters that do (Shopify, WooCommerce, THG hub detail endpoint)
// populate it. The retrieval engine treats variants as optional
// detail — they enrich a quoted product but the parent is the asset.
type ProductVariant struct {
	// SKU is the variant-level SKU. Required.
	SKU string

	// Title is a human-readable variant label ("Size M / Red"). May
	// be empty when the adapter cannot derive one; downstream code
	// then composes it from Attributes.
	Title string

	// Price bounds for this specific variant. Both nil = inherit from
	// parent. PriceMin == PriceMax for fixed-price variants.
	PriceMin *float64
	PriceMax *float64
	Currency string // ISO 4217 uppercase

	// ImageURL is the variant-specific hero image. Empty = use parent's
	// primary image.
	ImageURL string

	// Availability for this specific variant. AvailUnknown is fine.
	Availability Availability

	// Attributes is the free-form variant metadata — typical keys are
	// "size", "color", "material", "fabric". Adapters should not store
	// price/availability here (they have dedicated fields above).
	Attributes map[string]string
}
