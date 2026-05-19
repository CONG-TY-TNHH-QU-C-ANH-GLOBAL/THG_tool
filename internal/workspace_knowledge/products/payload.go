package products

import (
	"encoding/json"
	"time"
)

// PayloadSchemaVersion is the on-disk JSON schema version for product
// asset payloads. Bumped only on breaking changes — a v1 payload must
// remain readable forever (knowledge_assets rows are write-mostly), so
// new fields are added as optional and absorbed by older readers
// without a version bump. A version bump means a migration is owed.
const PayloadSchemaVersion = 1

// PayloadV1 is the JSON shape persisted in [assets.Asset.Payload] for
// product-catalog assets. It mirrors CanonicalProduct closely but is
// the on-disk contract — downstream retrieval queries read this shape
// (after json.Unmarshal), not the Go struct directly.
//
// Field tags are explicit because Go's default json encoder would
// emit zero-valued floats and strings; we want a compact, stable JSON
// where missing data shows up as absent keys (omitempty) and the
// schema version is always present.
type PayloadV1 struct {
	SchemaVersion int `json:"schema_version"`

	SourceID   string `json:"source_id"`
	DisplaySKU string `json:"display_sku,omitempty"`
	VendorSKU  string `json:"vendor_sku,omitempty"`

	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Origin      string `json:"origin,omitempty"`

	Sizes  []string `json:"sizes,omitempty"`
	Colors []string `json:"colors,omitempty"`
	Tags   []string `json:"tags,omitempty"`

	PriceMin *float64 `json:"price_min,omitempty"`
	PriceMax *float64 `json:"price_max,omitempty"`
	Currency string   `json:"currency,omitempty"`

	Images   []string         `json:"images,omitempty"`
	Variants []PayloadVariant `json:"variants,omitempty"`

	Availability Availability `json:"availability"`

	// Lineage block — always present so operator-replay reads can
	// rely on these keys existing even for older payloads.
	SourceURL        string    `json:"source_url,omitempty"`
	SourceUpdatedAt  time.Time `json:"source_updated_at"`
	RawPayloadHash   string    `json:"raw_payload_hash,omitempty"`
	ExtractorVersion string    `json:"extractor_version"`
}

// PayloadVariant is the persisted variant shape inside PayloadV1.
// Mirrors ProductVariant; kept separate so the Go struct can carry
// non-JSON-tagged conveniences (e.g. helper methods) without leaking
// into the on-disk schema.
type PayloadVariant struct {
	SKU          string            `json:"sku"`
	Title        string            `json:"title,omitempty"`
	PriceMin     *float64          `json:"price_min,omitempty"`
	PriceMax     *float64          `json:"price_max,omitempty"`
	Currency     string            `json:"currency,omitempty"`
	ImageURL     string            `json:"image_url,omitempty"`
	Availability Availability      `json:"availability"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// MarshalPayload returns the JSON bytes for one CanonicalProduct,
// ready to set as [assets.Asset.Payload]. The caller is responsible
// for having called Normalize / Validate first; MarshalPayload does
// not re-normalise (it should be deterministic given a normalised
// input).
func MarshalPayload(p *CanonicalProduct) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	v := PayloadV1{
		SchemaVersion:    PayloadSchemaVersion,
		SourceID:         p.SourceID,
		DisplaySKU:       p.DisplaySKU,
		VendorSKU:        p.VendorSKU,
		Name:             p.Name,
		Description:      p.Description,
		Category:         p.Category,
		Origin:           p.Origin,
		Sizes:            p.Sizes,
		Colors:           p.Colors,
		Tags:             p.Tags,
		PriceMin:         p.PriceMin,
		PriceMax:         p.PriceMax,
		Currency:         p.Currency,
		Images:           p.Images,
		Availability:     p.Availability,
		SourceURL:        p.SourceURL,
		SourceUpdatedAt:  p.SourceUpdatedAt.UTC(),
		RawPayloadHash:   p.RawPayloadHash,
		ExtractorVersion: p.ExtractorVersion,
	}
	if len(p.Variants) > 0 {
		v.Variants = make([]PayloadVariant, len(p.Variants))
		for i, src := range p.Variants {
			v.Variants[i] = PayloadVariant{
				SKU:          src.SKU,
				Title:        src.Title,
				PriceMin:     src.PriceMin,
				PriceMax:     src.PriceMax,
				Currency:     src.Currency,
				ImageURL:     src.ImageURL,
				Availability: src.Availability,
				Attributes:   src.Attributes,
			}
		}
	}
	return json.Marshal(v)
}
