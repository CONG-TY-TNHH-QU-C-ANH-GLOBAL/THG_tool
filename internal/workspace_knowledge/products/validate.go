package products

import (
	"errors"
	"fmt"
	"strings"
)

// validate.go holds the boundary-invariant checks (required fields, availability,
// currency shape, price range, variant SKUs, payload-hash hex) — the "pricing /
// currency" validation responsibility split out of the canonical god-file. Moved
// verbatim from canonical.go; same checks, order, and error strings.

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
	if err := p.validateHeader(); err != nil {
		return err
	}
	for i := range p.Variants {
		if err := validateVariant(i, &p.Variants[i]); err != nil {
			return err
		}
	}
	if p.RawPayloadHash != "" && !isHex(p.RawPayloadHash) {
		return fmt.Errorf("products: raw_payload_hash is not hex")
	}
	return nil
}

// validateHeader checks the product-level required fields, defaults a blank
// availability to AvailUnknown (in place), and validates the availability,
// currency, and price-range shape. Extracted verbatim from Validate — same
// checks, same order, same error strings, same in-place defaulting.
func (p *CanonicalProduct) validateHeader() error {
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
	return nil
}

// validateVariant checks one variant's SKU, defaults a blank availability to
// AvailUnknown (in place), and validates the availability, currency, and
// price-range shape. Extracted verbatim from Validate's variant loop body —
// same checks, same order, same indexed error strings, same in-place defaulting.
func validateVariant(i int, vr *ProductVariant) error {
	if strings.TrimSpace(vr.SKU) == "" {
		return fmt.Errorf("products: variant[%d].sku is required", i)
	}
	if vr.Availability == "" {
		vr.Availability = AvailUnknown
	}
	if !vr.Availability.IsKnown() {
		return fmt.Errorf("products: variant[%d] unknown availability %q", i, string(vr.Availability))
	}
	if vr.Currency != "" {
		cur := vr.Currency
		if len(cur) != 3 || strings.ToUpper(cur) != cur {
			return fmt.Errorf("products: variant[%d] currency %q is not a 3-letter uppercase code", i, cur)
		}
	}
	if vr.PriceMin != nil && vr.PriceMax != nil &&
		*vr.PriceMin > *vr.PriceMax {
		return fmt.Errorf("products: variant[%d] price_min > price_max", i)
	}
	return nil
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
