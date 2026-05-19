package products

import (
	"fmt"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// ToAsset maps a CanonicalProduct into [assets.Asset]. The asset is
// ready for [ingestion.AssetWriter.Write] — the writer will fill in
// OrgID and SourceID from its binding and validate the rest.
//
// Mapping rules (the single source of truth — every adapter goes
// through this function, so changing the mapping here automatically
// updates every adapter):
//
//   - Type        → assets.AssetPODProduct.
//   - ExternalID  → CanonicalProduct.SourceID (the upstream stable ID).
//   - Title       → CanonicalProduct.Name (or fallback "{Category} —
//                   {DisplaySKU}" / "Untitled product" so the asset
//                   always has a non-empty title for Validate).
//   - Description → CanonicalProduct.Description (untouched; agent
//                   prompt assembly decides truncation).
//   - Tags        → union of Category, Origin, Sizes, Colors, Tags,
//                   plus the Availability value. Normalised via
//                   assets.NormalizeTags so retrieval queries on tag
//                   match are case-/whitespace-stable.
//   - Payload     → MarshalPayload(p): the on-disk JSON schema. Carries
//                   structured fields retrieval needs (price, images,
//                   variants, lineage) that don't fit into Tags.
//   - State       → assets.StatePending. Operator approves explicitly;
//                   ingestion never auto-approves.
//
// The returned Asset has OrgID and SourceID zero — the AssetWriter
// binding sets them. Callers MUST NOT set them themselves: the writer
// rejects (ErrCrossTenantWrite) any asset where they are pre-filled
// to a value other than the writer's binding.
func ToAsset(p *CanonicalProduct) (*assets.Asset, error) {
	if p == nil {
		return nil, fmt.Errorf("products: nil product")
	}
	payload, err := MarshalPayload(p)
	if err != nil {
		return nil, fmt.Errorf("products: marshal payload: %w", err)
	}
	title := strings.TrimSpace(p.Name)
	if title == "" {
		// Pick a reasonable fallback so assets.Asset.Validate() does not
		// reject the row. An adapter that triggers this branch has a
		// data-quality problem; the operator UI will surface "Untitled
		// product" rows for review.
		switch {
		case p.DisplaySKU != "" && p.Category != "":
			title = p.Category + " — " + p.DisplaySKU
		case p.DisplaySKU != "":
			title = p.DisplaySKU
		case p.Category != "":
			title = p.Category
		default:
			title = "Untitled product"
		}
	}
	tags := buildTags(p)
	return &assets.Asset{
		Type:        assets.AssetPODProduct,
		ExternalID:  p.SourceID,
		Title:       title,
		Description: p.Description,
		Tags:        tags,
		Payload:     payload,
		State:       assets.StatePending,
	}, nil
}

// buildTags unions every retrieval-discriminating string into a tag
// list. Order does not matter — assets.NormalizeTags sorts.
//
// What goes in: category, origin, every size, every color, every
// free-form tag, and the availability value (so a retrieval query
// can filter to in_stock products with a tag predicate alone).
func buildTags(p *CanonicalProduct) []string {
	raw := make([]string, 0, 4+len(p.Sizes)+len(p.Colors)+len(p.Tags))
	if p.Category != "" {
		raw = append(raw, p.Category)
	}
	if p.Origin != "" {
		raw = append(raw, p.Origin)
	}
	raw = append(raw, p.Sizes...)
	raw = append(raw, p.Colors...)
	raw = append(raw, p.Tags...)
	if p.Availability != "" {
		raw = append(raw, string(p.Availability))
	}
	return assets.NormalizeTags(raw)
}
