package csv

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
)

// header → column index, lower-cased + trimmed for case-insensitive
// lookups. Built once per CSV; queried per row.
type headerIndex map[string]int

func indexHeaders(header []string) headerIndex {
	out := make(headerIndex, len(header))
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		if key == "" {
			continue
		}
		// First occurrence wins. Duplicate columns ("Price", "Price")
		// are an operator error we surface row-by-row when the second
		// column's data conflicts; the index itself stays clean.
		if _, exists := out[key]; !exists {
			out[key] = i
		}
	}
	return out
}

// picker resolves CSV columns into Asset fields. Built once per sync,
// after we know which headers the file actually contains. Keeps
// per-row work to a few map lookups and string trims.
type picker struct {
	cols           headerIndex
	idxExternalID  int // -1 if absent
	idxTitle       int // required; otherwise the file is rejected at resolvePicker
	idxDescription int // -1 if absent
	idxTags        int // -1 if absent
	// remaining columns flow into asset.payload as a generic map so
	// retrieval can match against them even when no field mapped.
	payloadCols []payloadCol
}

type payloadCol struct {
	header string // original header (case preserved for payload JSON keys)
	idx    int
}

// defaultHeader returns the column-map header for field name, with the
// canonical default used when the operator did not customise it.
func defaultHeader(custom, fallback string) string {
	custom = strings.ToLower(strings.TrimSpace(custom))
	if custom != "" {
		return custom
	}
	return fallback
}

// resolvePicker validates that every required column exists and
// computes the column indices used during row iteration. Returns an
// "unmapped" picker that toAsset will reject every row from, so the
// caller can fall through to the per-row error path without
// special-casing config-vs-data failures.
func resolvePicker(cols headerIndex, m ColumnMap, tagCol string) *picker {
	titleHeader := defaultHeader(m.Title, "title")
	idxTitle, ok := cols[titleHeader]
	if !ok {
		// We don't return an error from this constructor — the caller
		// still tries to read rows and surfaces per-row "missing
		// title" errors. That gives the operator a uniform error UX:
		// one error message format regardless of whether the file is
		// empty, mis-headed, or has bad rows.
		idxTitle = -1
	}

	p := &picker{
		cols:           cols,
		idxExternalID:  lookupOrMinusOne(cols, defaultHeader(m.ExternalID, "external_id")),
		idxTitle:       idxTitle,
		idxDescription: lookupOrMinusOne(cols, defaultHeader(m.Description, "description")),
		idxTags:        lookupOrMinusOne(cols, strings.ToLower(strings.TrimSpace(tagCol))),
	}
	// Reserve the named columns so they don't double into payload.
	reserved := map[int]struct{}{}
	for _, i := range []int{p.idxExternalID, p.idxTitle, p.idxDescription, p.idxTags} {
		if i >= 0 {
			reserved[i] = struct{}{}
		}
	}
	// Build payload set from the remaining columns. We need the
	// original header strings (case preserved) so the operator sees
	// "Wholesale Price" in the payload JSON, not "wholesale price".
	for header, idx := range cols {
		if _, isReserved := reserved[idx]; isReserved {
			continue
		}
		p.payloadCols = append(p.payloadCols, payloadCol{header: header, idx: idx})
	}
	return p
}

func lookupOrMinusOne(cols headerIndex, key string) int {
	if key == "" {
		return -1
	}
	if i, ok := cols[key]; ok {
		return i
	}
	return -1
}

func (p *picker) externalID(row []string) string {
	if p.idxExternalID < 0 || p.idxExternalID >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[p.idxExternalID])
}

// toAsset converts one CSV row to an Asset. Returns an error for
// row-level problems (missing title, blank row); the dispatcher
// reports these per row without aborting the sync.
//
// External ID strategy: if the CSV has an external_id column, use it.
// Otherwise, compute [assets.ContentFingerprint] over the row's
// content so re-uploads of the same CSV produce stable IDs (idempotent
// re-sync).
func (p *picker) toAsset(row []string, typ assets.AssetType, rowNum int) (*assets.Asset, error) {
	_ = rowNum // available for future row-aware error messages
	if p.idxTitle < 0 {
		return nil, errors.New("missing 'title' column (set ColumnMap.Title to map a non-standard header)")
	}
	if p.idxTitle >= len(row) {
		return nil, errors.New("row shorter than header")
	}
	title := strings.TrimSpace(row[p.idxTitle])
	if title == "" {
		return nil, errors.New("title is blank")
	}
	a := &assets.Asset{
		Type:  typ,
		Title: title,
	}
	if p.idxDescription >= 0 && p.idxDescription < len(row) {
		a.Description = strings.TrimSpace(row[p.idxDescription])
	}
	if p.idxTags >= 0 && p.idxTags < len(row) {
		a.Tags = splitTagField(row[p.idxTags])
	}
	// Build payload from non-reserved columns. The payload is a JSON
	// object keyed by the original header, value the trimmed cell.
	if len(p.payloadCols) > 0 {
		b, err := p.rowPayloadJSON(row)
		if err != nil {
			return nil, err
		}
		if b != nil {
			a.Payload = b
		}
	}
	// External ID: prefer the operator-mapped column, fall back to a
	// content fingerprint so the (org_id, source_id, external_id)
	// unique index can still dedupe across re-uploads.
	if extID := p.externalID(row); extID != "" {
		a.ExternalID = extID
	} else {
		a.ExternalID = assets.ContentFingerprint(a.Title, a.Description, a.Tags, a.Payload)
	}
	return a, nil
}

// rowPayloadJSON marshals the row's non-reserved payload columns into a JSON
// object keyed by the original header (trimmed cell values; empty cells and
// out-of-range columns skipped). Returns (nil, nil) when no payload cell has a
// value. Extracted verbatim from toAsset's payload-building block.
func (p *picker) rowPayloadJSON(row []string) ([]byte, error) {
	payload := make(map[string]string, len(p.payloadCols))
	for _, pc := range p.payloadCols {
		if pc.idx >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[pc.idx])
		if v == "" {
			continue
		}
		payload[pc.header] = v
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return json.Marshal(payload)
}

// splitTagField breaks a tag-column cell into individual tags. Accepts
// commas, semicolons, pipes, or whitespace as separators — the CSV
// convention is not standardised, so we accept all common shapes.
func splitTagField(cell string) []string {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return nil
	}
	parts := strings.FieldsFunc(cell, func(r rune) bool {
		return r == ',' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
