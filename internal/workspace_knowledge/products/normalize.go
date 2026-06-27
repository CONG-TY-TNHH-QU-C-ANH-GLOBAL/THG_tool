package products

import (
	"sort"
	"strings"
)

// normalize.go holds the canonicalisation helpers split out of the canonical
// god-file. Moved verbatim from canonical.go; the idempotence guarantee
// (two re-ingests → byte-identical JSON) is unchanged.

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
