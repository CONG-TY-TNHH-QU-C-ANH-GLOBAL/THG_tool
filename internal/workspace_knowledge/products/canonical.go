package products

import "time"

// CanonicalProduct is the vendor-neutral product representation every
// adapter emits. It is the only product shape downstream code knows
// about — retrieval, governance filters, agent grounding, operator
// review UI all read this struct (or its persisted JSON payload).
//
// The contract is intentionally lean: ~20 fields, all derivable from
// every catalog backend the platform supports. Fields adapters cannot
// populate stay at their zero value; Validate() decides which zero
// values are acceptable.
//
// Behaviour lives in siblings (same package): availability.go (the
// Availability enum + ProductVariant), validate.go (Validate +
// boundary invariants), normalize.go (Normalize + canonicalisation).
type CanonicalProduct struct {
	// SourceID is the stable identifier from the upstream system. It
	// becomes [assets.Asset.ExternalID] and is the key for idempotent
	// re-ingest — a re-sync that returns the same SourceID UPDATEs the
	// existing asset; a different SourceID INSERTs a new one. Adapters
	// MUST choose a SourceID that survives upstream restarts (cuid /
	// uuid / database PK), never something synthetic like array index.
	SourceID string

	// DisplaySKU is the SKU shown to humans in the operator UI and
	// embedded in agent comments. THG calls this thgSku; Shopify
	// surfaces it as SKU; CSVs typically have one column for it.
	DisplaySKU string

	// VendorSKU is the upstream-internal SKU. Optional. When the
	// upstream system has only one SKU, leave this empty and put it
	// in DisplaySKU.
	VendorSKU string

	Name        string
	Description string

	// Category is a single free-form category label (Apparel, Phone
	// Cases, Home & Living, …). Adapters that have a category tree
	// MAY pick the leaf; the retrieval engine does not currently
	// understand hierarchy.
	Category string

	// Origin is the country/region the product is fulfilled from.
	// Stored as a free-form short code uppercase (VN, US, CN, EU,
	// "GLOBAL"). Not strictly ISO-3166: catalogs in the wild emit
	// non-ISO regions and forcing the enum drops data.
	Origin string

	// Sizes and Colors are derived sets across all variants. When
	// Variants is empty these may still be populated (catalogs that
	// flatten variants into a single SKU). Normalised by Normalize:
	// lower-cased, deduped, sorted.
	Sizes  []string
	Colors []string

	// Tags is free-form labels the adapter chose to surface for
	// retrieval — design style ("anime", "streetwear"), seasonal flags
	// ("xmas", "halloween"), audience hints ("kidswear"). Independent
	// of Category and Sizes/Colors. Normalised by Normalize.
	Tags []string

	// Price bounds for the parent product (across variants if any).
	// nil = unknown. PriceMin == PriceMax for fixed-price products.
	PriceMin *float64
	PriceMax *float64
	Currency string // ISO 4217 uppercase

	// Images is the ordered list of image URLs. Images[0] is the
	// primary. URLs only — ingestion NEVER downloads images; the
	// agent layer embeds URLs directly in outbound comments.
	Images []string

	// Variants is the optional structured variant breakdown. Empty
	// when the adapter has no variant data.
	Variants []ProductVariant

	// Availability rolls up across variants. If any variant is in
	// stock the parent is AvailInStock; if all discontinued, the
	// parent is AvailDiscontinued; otherwise the adapter's best
	// guess (or AvailUnknown).
	Availability Availability

	// ── Lineage / replay-stability ─────────────────────────────────

	// SourceURL is the canonical URL of the product on the upstream
	// system (PDP link). Empty when the upstream does not expose one.
	SourceURL string

	// SourceUpdatedAt is the upstream's own updated_at, NOT the time
	// of our ingest. Used by the scheduler to skip unchanged rows
	// and by operators to answer "is the catalog stale?".
	SourceUpdatedAt time.Time

	// RawPayloadHash is the sha256 (hex) of the raw upstream payload
	// that produced this row. Adapters compute it from the unmodified
	// response bytes for the specific item — not the whole response.
	RawPayloadHash string

	// ExtractorVersion is the adapter's own identifier and version,
	// e.g. "rest_json/v1" or "shopify/v3". Bumped when the field-
	// extraction logic changes so historical assets can be re-
	// extracted for replay.
	ExtractorVersion string
}
