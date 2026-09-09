// Package suppliers owns the persisted payload schema for a sourceable
// marketplace item (1688 / Taobao). It is the supplier-side counterpart of
// package products: products describes what the org sells, suppliers describes
// what THG can source and fulfil on the customer's behalf.
//
// The package holds no IO and no store imports — the ingestor builds the
// payload, the retrieval layer reads it back.
package suppliers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// SchemaVersion is the current payload version. Bump it only for a
// backward-incompatible field change; readers tolerate absent keys.
const SchemaVersion = 1

// ExtractorVersion identifies the mapping that produced a payload, so a
// re-index can tell rows built by an older mapping apart.
const ExtractorVersion = "suppliers/v1"

// PriceTier is one MOQ break on a wholesale offer. 1688 quotes by volume;
// Taobao items normally carry a single tier or none at all.
type PriceTier struct {
	MOQ   int     `json:"moq"`
	Price float64 `json:"price"`
}

// PayloadV1 is the persisted shape. Pointer fields are absent-when-unknown on
// purpose: a missing weight must read as "not known", never as zero, because
// the operator-facing message quotes these numbers verbatim.
type PayloadV1 struct {
	SchemaVersion int `json:"schema_version"`

	Platform  string `json:"platform"` // taobao | alibaba
	ProductID string `json:"product_id"`

	Title    string `json:"title"`
	TitleCN  string `json:"title_cn,omitempty"`
	ShopName string `json:"shop_name,omitempty"`
	Category string `json:"category,omitempty"`

	PriceCNY   *float64    `json:"price_cny,omitempty"`
	PriceNote  string      `json:"price_note,omitempty"`
	PriceTiers []PriceTier `json:"price_tiers,omitempty"`
	MOQ        *int        `json:"moq,omitempty"`
	Unit       string      `json:"unit,omitempty"`

	WeightKG *float64 `json:"weight_kg,omitempty"`
	LengthCM *float64 `json:"length_cm,omitempty"`
	WidthCM  *float64 `json:"width_cm,omitempty"`
	HeightCM *float64 `json:"height_cm,omitempty"`

	ShipFrom  string `json:"ship_from,omitempty"`
	SoldCount *int   `json:"sold_count,omitempty"`

	Images []string `json:"images,omitempty"`

	SourceURL        string    `json:"source_url,omitempty"`
	SourceFetchedAt  time.Time `json:"source_fetched_at"`
	ExtractorVersion string    `json:"extractor_version"`
}

// ExternalID is the stable per-source identity used for idempotent re-ingest.
func (p PayloadV1) ExternalID() string {
	return strings.TrimSpace(p.Platform) + ":" + strings.TrimSpace(p.ProductID)
}

// Asset renders the payload as a knowledge asset. State is left to the caller
// (the ingestor writes pending; the operator approves) — this function only
// builds the ingestor-controlled fields.
//
// Returns ok=false when the payload lacks the identity or the source link the
// downstream message needs, so a half-resolved product never becomes a
// retrievable asset.
func (p PayloadV1) Asset(orgID, sourceID int64, tags []string) (*assets.Asset, bool) {
	if orgID <= 0 || sourceID <= 0 {
		return nil, false
	}
	if strings.TrimSpace(p.Platform) == "" || strings.TrimSpace(p.ProductID) == "" {
		return nil, false
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = strings.TrimSpace(p.TitleCN)
	}
	if title == "" || strings.TrimSpace(p.SourceURL) == "" {
		return nil, false
	}
	p.SchemaVersion = SchemaVersion
	p.ExtractorVersion = ExtractorVersion
	body, err := json.Marshal(p)
	if err != nil {
		return nil, false
	}
	return &assets.Asset{
		OrgID:       orgID,
		SourceID:    sourceID,
		ExternalID:  p.ExternalID(),
		Type:        assets.AssetSupplierProduct,
		Title:       title,
		Description: p.description(),
		Tags:        assets.NormalizeTags(append([]string{p.Platform}, tags...)),
		Payload:     body,
	}, true
}

// description is the retrievable prose for the asset. Keyword search matches on
// it, so it carries the Chinese title and the shop/origin words a lead's post
// is unlikely to contain in the translated title alone.
func (p PayloadV1) description() string {
	parts := make([]string, 0, 4)
	for _, candidate := range []string{p.TitleCN, p.Category, p.ShopName, p.ShipFrom} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, " · ")
}
