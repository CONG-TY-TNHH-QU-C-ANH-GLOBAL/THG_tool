package products

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

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

// CanonicalProduct is the vendor-neutral product representation every
// adapter emits. It is the only product shape downstream code knows
// about — retrieval, governance filters, agent grounding, operator
// review UI all read this struct (or its persisted JSON payload).
//
// The contract is intentionally lean: ~20 fields, all derivable from
// every catalog backend the platform supports. Fields adapters cannot
// populate stay at their zero value; Validate() decides which zero
// values are acceptable.
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

// Validate enforces the boundary invariants every adapter must meet.
// Called once per product before persistence. Adapters that produce
// invalid products should surface them via SyncResult.Errors rather
// than panic — Validate returning an error is data quality feedback,
// not a programmer mistake.
//
// The rules:
//
//   - SourceID, Name, ExtractorVersion are required (cannot be empty).
//   - SourceUpdatedAt must be non-zero. An adapter that has no upstream
//     timestamp MUST fall back to the ingest wall-clock and document
//     that decision in its own normalisation step. We refuse to let
//     adapters silently emit zero timestamps because that breaks the
//     "is this stale?" operator question.
//   - Availability must be a known value. AvailUnknown is fine; "".
//   - Currency, if set, must be 3 uppercase letters (ISO 4217 shape).
//   - PriceMin <= PriceMax when both are set.
//   - Each variant must have a non-empty SKU.
//   - RawPayloadHash, if set, must be hex.
//
// Validate does NOT cross-check that Sizes/Colors derive from Variants —
// adapters may emit one or the other depending on the upstream shape.
func (p *CanonicalProduct) Validate() error {
	if p == nil {
		return errors.New("products: nil product")
	}
	if strings.TrimSpace(p.SourceID) == "" {
		return errors.New("products: source_id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("products: name is required")
	}
	if strings.TrimSpace(p.ExtractorVersion) == "" {
		return errors.New("products: extractor_version is required")
	}
	if p.SourceUpdatedAt.IsZero() {
		return errors.New("products: source_updated_at must be non-zero")
	}
	if p.Availability == "" {
		p.Availability = AvailUnknown
	}
	if !p.Availability.IsKnown() {
		return fmt.Errorf("products: unknown availability %q", string(p.Availability))
	}
	if p.Currency != "" {
		if len(p.Currency) != 3 || strings.ToUpper(p.Currency) != p.Currency {
			return fmt.Errorf("products: currency %q is not a 3-letter uppercase code", p.Currency)
		}
	}
	if p.PriceMin != nil && p.PriceMax != nil && *p.PriceMin > *p.PriceMax {
		return fmt.Errorf("products: price_min %v > price_max %v", *p.PriceMin, *p.PriceMax)
	}
	for i := range p.Variants {
		if strings.TrimSpace(p.Variants[i].SKU) == "" {
			return fmt.Errorf("products: variant[%d].sku is required", i)
		}
		if p.Variants[i].Availability == "" {
			p.Variants[i].Availability = AvailUnknown
		}
		if !p.Variants[i].Availability.IsKnown() {
			return fmt.Errorf("products: variant[%d] unknown availability %q", i, string(p.Variants[i].Availability))
		}
		if p.Variants[i].Currency != "" {
			cur := p.Variants[i].Currency
			if len(cur) != 3 || strings.ToUpper(cur) != cur {
				return fmt.Errorf("products: variant[%d] currency %q is not a 3-letter uppercase code", i, cur)
			}
		}
		if p.Variants[i].PriceMin != nil && p.Variants[i].PriceMax != nil &&
			*p.Variants[i].PriceMin > *p.Variants[i].PriceMax {
			return fmt.Errorf("products: variant[%d] price_min > price_max", i)
		}
	}
	if p.RawPayloadHash != "" && !isHex(p.RawPayloadHash) {
		return fmt.Errorf("products: raw_payload_hash is not hex")
	}
	return nil
}

// Normalize canonicalises the in-memory product before persistence.
// Adapters SHOULD call Normalize as the last step of their per-item
// extraction so the persisted shape is stable across re-ingests.
//
// Normalisation rules:
//
//   - All strings trimmed of leading/trailing whitespace.
//   - Sizes, Colors, Tags lowercased, deduped (case-insensitive),
//     empty entries dropped, sorted alphabetically.
//   - Currency uppercased.
//   - Origin uppercased.
//   - Images de-duplicated preserving order (first occurrence wins).
//   - Variant Currency uppercased; Variant Attributes keys lowercased.
//
// Normalize is idempotent: calling it twice yields the same result.
// This is load-bearing for the "two re-ingests of the same upstream
// row produce byte-identical canonical JSON" guarantee that the hash-
// based change detection relies on.
func (p *CanonicalProduct) Normalize() {
	if p == nil {
		return
	}
	p.SourceID = strings.TrimSpace(p.SourceID)
	p.DisplaySKU = strings.TrimSpace(p.DisplaySKU)
	p.VendorSKU = strings.TrimSpace(p.VendorSKU)
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Category = strings.TrimSpace(p.Category)
	p.Origin = strings.ToUpper(strings.TrimSpace(p.Origin))
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	p.SourceURL = strings.TrimSpace(p.SourceURL)
	p.ExtractorVersion = strings.TrimSpace(p.ExtractorVersion)
	p.RawPayloadHash = strings.ToLower(strings.TrimSpace(p.RawPayloadHash))

	p.Sizes = normaliseStringSet(p.Sizes)
	p.Colors = normaliseStringSet(p.Colors)
	p.Tags = normaliseStringSet(p.Tags)
	p.Images = dedupePreservingOrder(p.Images)

	for i := range p.Variants {
		p.Variants[i].SKU = strings.TrimSpace(p.Variants[i].SKU)
		p.Variants[i].Title = strings.TrimSpace(p.Variants[i].Title)
		p.Variants[i].Currency = strings.ToUpper(strings.TrimSpace(p.Variants[i].Currency))
		p.Variants[i].ImageURL = strings.TrimSpace(p.Variants[i].ImageURL)
		if p.Variants[i].Attributes != nil {
			lower := make(map[string]string, len(p.Variants[i].Attributes))
			for k, v := range p.Variants[i].Attributes {
				lk := strings.ToLower(strings.TrimSpace(k))
				if lk == "" {
					continue
				}
				lower[lk] = strings.TrimSpace(v)
			}
			p.Variants[i].Attributes = lower
		}
	}
}

// normaliseStringSet lowercases, trims, drops empties, dedupes, and
// sorts. Same algorithm as assets.NormalizeTags but kept here so the
// products package does not have to round-trip a tag-typed slice for
// non-tag fields like Sizes/Colors.
func normaliseStringSet(raw []string) []string {
	if len(raw) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		clean := strings.ToLower(strings.TrimSpace(s))
		if clean == "" {
			continue
		}
		if _, dup := seen[clean]; dup {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

// dedupePreservingOrder removes duplicate strings keeping the first
// occurrence. Used for Images where order is meaningful (Images[0] is
// the hero) but duplicates from upstream still need to be collapsed.
func dedupePreservingOrder(raw []string) []string {
	if len(raw) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		t := strings.TrimSpace(s)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return s != ""
}
