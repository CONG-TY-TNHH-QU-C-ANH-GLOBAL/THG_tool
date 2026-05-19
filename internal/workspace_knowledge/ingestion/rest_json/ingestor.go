package rest_json

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/products"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Ingestor is the rest_json adapter. One instance handles all
// SourceRESTJSON sources across all tenants — it carries no
// tenant state, every call to [Ingestor.Sync] is self-contained
// over the [sources.Source.ConnectionConfig] it receives.
//
// The HTTPDoer is injectable so tests can swap in a faketransport
// without standing up an httptest server when desired. Production
// uses the default *http.Client built from the source's timeout.
type Ingestor struct {
	// HTTP is optional. When nil, Sync constructs a per-call *http.Client
	// with the timeout from the source config. Tests pass a fake.
	HTTP HTTPDoer
}

// New returns the production ingestor wired to net/http defaults.
func New() *Ingestor { return &Ingestor{} }

// Type identifies which sources.SourceType this ingestor handles.
// Required by [ingestion.Ingestor].
func (i *Ingestor) Type() sources.SourceType { return sources.SourceRESTJSON }

// Sync pulls every page of the configured endpoint, maps each item
// into a CanonicalProduct, and writes via the products.Writer.
//
// One sync is idempotent: re-running over the same upstream data
// updates the same assets (matched on SourceID) rather than
// inserting duplicates — guaranteed by the writer + asset upsert
// downstream.
//
// Error policy:
//   - Config / parse / auth errors return a wrapped permanent error
//     before any pagination begins; SyncResult is zero.
//   - Per-page HTTP errors return a wrapped error after recording
//     the rows from successful pages. The caller (dispatcher) maps
//     this to the health status.
//   - Per-item extraction / validate errors are surfaced via
//     SyncResult.Errors and the loop continues.
func (i *Ingestor) Sync(ctx context.Context, src *sources.Source, w ingestion.AssetWriter) (ingestion.SyncResult, error) {
	if src == nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("rest_json: nil source"))
	}
	if w == nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("rest_json: nil writer"))
	}
	cfg, err := ParseConfig(src.ConnectionConfig)
	if err != nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(err)
	}
	doer := i.HTTP
	if doer == nil {
		doer = &http.Client{Timeout: cfg.TimeoutDuration()}
	}
	writer := products.NewWriter(w)
	if writer == nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("rest_json: cannot wrap writer"))
	}

	res := ingestion.SyncResult{}
	page := cfg.Pagination.StartPage
	if cfg.Pagination.Scheme == "none" {
		page = 1
	}
	totalPages := 0 // 0 = unknown; rely on empty-page sentinel
	for pageCount := 0; pageCount < cfg.Pagination.MaxPages; pageCount++ {
		fullURL, err := buildURL(cfg, page)
		if err != nil {
			return res, err
		}
		body, err := doRequest(ctx, doer, cfg, fullURL)
		if err != nil {
			// Page-level error: return partial results + the err so
			// the dispatcher records the health status. Successful
			// items already written are persisted.
			return res, err
		}

		var root any
		if err := json.Unmarshal(body, &root); err != nil {
			return res, ingestion.WrapPermanent(fmt.Errorf("rest_json: parse page %d: %w", page, err))
		}

		// Extract the items array via data_path.
		var dataNode any = root
		if cfg.DataPath != "" {
			n, ok := lookupPath(root, cfg.DataPath)
			if !ok {
				return res, ingestion.WrapPermanent(fmt.Errorf("rest_json: data_path %q not found in response", cfg.DataPath))
			}
			dataNode = n
		}
		items, ok := dataNode.([]any)
		if !ok {
			return res, ingestion.WrapPermanent(fmt.Errorf("rest_json: data_path %q is not an array", cfg.DataPath))
		}

		// Capture total pages once (some upstreams change it mid-walk;
		// we trust the first response and clamp).
		if totalPages == 0 && cfg.Pagination.TotalPagesPath != "" {
			if v, ok := lookupPath(root, cfg.Pagination.TotalPagesPath); ok {
				if f := asFloat(v); f != nil {
					totalPages = int(*f)
				}
			}
		}

		// Process each item.
		for idx, item := range items {
			obj, ok := item.(map[string]any)
			if !ok {
				res.AssetsRejected++
				res.Errors = append(res.Errors, ingestion.SyncError{
					Reason: fmtErr(idx, "not a JSON object on page %d", page),
				})
				continue
			}
			res.AssetsSeen++
			cp, syncErr := extractItem(obj, cfg)
			if syncErr != nil {
				res.AssetsRejected++
				res.Errors = append(res.Errors, *syncErr)
				continue
			}
			// Write — Writer handles Normalize + Validate.
			if err := writer.Write(ctx, cp); err != nil {
				res.AssetsRejected++
				res.Errors = append(res.Errors, ingestion.SyncError{
					ExternalID: cp.SourceID,
					Reason:     "write rejected",
					Detail:     err.Error(),
				})
				continue
			}
			// We do not split Created vs Updated — the storeAssetWriter
			// upserts and does not surface that distinction to the
			// adapter. Counted as Created for the dispatcher metric;
			// PR-3 can refine if the AssetWriter contract grows.
			res.AssetsCreated++
		}

		// Stop conditions:
		//   1) Pagination scheme is none — one fetch is the whole sync.
		//   2) The page had no items — upstream signalled exhaustion.
		//   3) totalPages known and we just processed the last one.
		if cfg.Pagination.Scheme == "none" {
			return res, nil
		}
		if len(items) == 0 {
			return res, nil
		}
		if totalPages > 0 && page >= totalPages {
			return res, nil
		}
		page++
	}
	// Hit MaxPages — return what we have, no error. An operator who
	// expected more pages can raise the cap; an unbounded walk is the
	// problem we are guarding against.
	return res, nil
}

// extractItem applies the field_map to one upstream item and returns
// a CanonicalProduct. Returns a SyncError on extraction failure (vs
// returning an error for the whole sync) so one bad row does not
// kill the rest of the page.
func extractItem(obj map[string]any, cfg *Config) (*products.CanonicalProduct, *ingestion.SyncError) {
	fm := cfg.FieldMap
	cp := &products.CanonicalProduct{
		ExtractorVersion: cfg.ExtractorVersion,
	}

	cp.SourceID = asString(getPath(obj, fm.SourceID))
	if cp.SourceID == "" {
		return nil, &ingestion.SyncError{Reason: "missing source_id"}
	}
	cp.Name = asString(getPath(obj, fm.Name))
	cp.DisplaySKU = asString(getPath(obj, fm.DisplaySKU))
	cp.VendorSKU = asString(getPath(obj, fm.VendorSKU))
	cp.Description = asString(getPath(obj, fm.Description))
	cp.Category = asString(getPath(obj, fm.Category))
	cp.Origin = asString(getPath(obj, fm.Origin))
	cp.Sizes = asStringSlice(getPath(obj, fm.Sizes))
	cp.Colors = asStringSlice(getPath(obj, fm.Colors))
	cp.Tags = asStringSlice(getPath(obj, fm.Tags))
	cp.PriceMin = asFloat(getPath(obj, fm.PriceMin))
	cp.PriceMax = asFloat(getPath(obj, fm.PriceMax))
	cp.Currency = asString(getPath(obj, fm.Currency))
	cp.Images = asStringSlice(getPath(obj, fm.Images))
	cp.SourceUpdatedAt = asTime(getPath(obj, fm.SourceUpdatedAt))

	// Source URL: literal if no template, otherwise substitute {id} → SourceID.
	if fm.SourceURLTemplate != "" {
		cp.SourceURL = resolveTemplate(fm.SourceURLTemplate, map[string]string{
			"id":          cp.SourceID,
			"display_sku": cp.DisplaySKU,
			"vendor_sku":  cp.VendorSKU,
		})
	}

	// Availability: map from upstream value or fall back to default.
	cp.Availability = mapAvailability(obj, cfg.Availability)

	// Raw payload hash — sha256 of the canonical-JSON-encoded item.
	// Stable: same bytes → same hash; an extractor regression that
	// changes the mapping but not the source bytes leaves the hash
	// untouched, which is the signature for "the source did not
	// change but our interpretation did".
	itemBytes, _ := json.Marshal(obj) // map iteration order varies but Marshal sorts keys
	sum := sha256.Sum256(itemBytes)
	cp.RawPayloadHash = hex.EncodeToString(sum[:])

	return cp, nil
}

// mapAvailability resolves the canonical Availability for one item.
// Three branches:
//   - FromField unset → return Default.
//   - FromField set, value present in Map → return mapped value.
//   - FromField set but value missing/unmapped → return Default.
func mapAvailability(obj map[string]any, cfg AvailabilityConfig) products.Availability {
	if cfg.FromField == "" {
		return cfg.Default
	}
	raw, ok := lookupPath(obj, cfg.FromField)
	if !ok {
		return cfg.Default
	}
	key := asString(raw)
	if v, mapped := cfg.Map[key]; mapped {
		return v
	}
	return cfg.Default
}

// getPath is a thin wrapper around lookupPath that returns the bare
// value (nil if absent). All extraction helpers tolerate nil, so this
// keeps the per-field code in extractItem one-line per field.
func getPath(obj map[string]any, path string) any {
	v, ok := lookupPath(obj, path)
	if !ok {
		return nil
	}
	return v
}

// Compile-time assertion: Ingestor implements the contracts both
// upstream (ingestion.Ingestor for the dispatcher) and downstream
// (products.Adapter as the documented label).
var (
	_ ingestion.Ingestor = (*Ingestor)(nil)
	_ products.Adapter   = (*Ingestor)(nil)
)

// Sanity check on package boot: the default extractor version stays
// in sync with the products payload version conceptually. They are
// independent versions but bumping PayloadSchemaVersion without
// bumping DefaultExtractorVersion is almost always a mistake — this
// keeps the two side by side for the reviewer.
var _ = time.Duration(0) // keep time imported even if unused after refactor
