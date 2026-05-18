// Package csv implements the CSV ingestor for the Workspace
// Knowledge OS. A source of type "csv" carries the parsed CSV body
// inside its ConnectionConfig.payload — the operator uploaded it
// through the Sources panel UI and the upload handler stored the body
// as part of the config.
//
// This package converts CSV rows into [assets.Asset] values and
// writes them via the [ingestion.AssetWriter] the dispatcher
// supplied.
package csv

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/assets"
	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

// Ingestor implements [ingestion.Ingestor] for csv sources. See the
// package doc for the input contract. Designed for synchronous,
// in-memory CSVs up to a few MB — for larger uploads, the caller
// should stream rows directly and skip this implementation.
type Ingestor struct{}

// New returns a CSV ingestor. There is no configuration — the per-
// source configuration is read from src.ConnectionConfig.
func New() *Ingestor { return &Ingestor{} }

// Type satisfies [ingestion.Ingestor].
func (i *Ingestor) Type() sources.SourceType { return sources.SourceCSV }

// Config is the JSON shape stored in Source.ConnectionConfig for csv
// sources. The upload handler writes it; this ingestor reads it.
//
// AssetType controls how every row is interpreted (a CSV upload is a
// single asset type — mixing types within one CSV is not supported;
// operator should split into multiple sources).
//
// ColumnMap lets a CSV with non-standard headers ("Product Name",
// "Item Description", "Selling Price") map to the asset fields. Each
// value is the CSV column header that holds that field. Empty values
// fall back to the default header names below.
//
// Body is the actual CSV text. Stored inside the config so the
// ingestor is stateless w.r.t. filesystem and can be re-run from a
// scheduler without keeping the upload around.
type Config struct {
	AssetType assets.AssetType `json:"asset_type"`
	ColumnMap ColumnMap        `json:"column_map"`
	TagColumn string           `json:"tag_column"`
	Body      string           `json:"body"`
}

// ColumnMap maps Asset fields to CSV column headers. Empty fields use
// the defaults: "title", "description", "external_id". The mapper
// matches case-insensitively after trimming.
type ColumnMap struct {
	ExternalID  string `json:"external_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Sync reads the CSV body, normalizes each row into an Asset, and
// writes it via w. Tag and payload columns beyond the mapped fields
// flow into the asset's payload JSON so retrieval can still match
// against them.
func (i *Ingestor) Sync(ctx context.Context, src *sources.Source, w ingestion.AssetWriter) (ingestion.SyncResult, error) {
	if src == nil {
		return ingestion.SyncResult{}, errors.New("csv: source is nil")
	}
	var cfg Config
	if len(src.ConnectionConfig) == 0 {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("csv: connection_config is empty"))
	}
	if err := json.Unmarshal(src.ConnectionConfig, &cfg); err != nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(fmt.Errorf("csv: parse config: %w", err))
	}
	if !cfg.AssetType.IsKnown() {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(fmt.Errorf("csv: unknown asset_type %q", cfg.AssetType))
	}
	if strings.TrimSpace(cfg.Body) == "" {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New("csv: body is empty"))
	}

	reader := csv.NewReader(strings.NewReader(cfg.Body))
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return ingestion.SyncResult{}, ingestion.WrapPermanent(fmt.Errorf("csv: read header: %w", err))
	}
	cols := indexHeaders(header)
	pick := resolvePicker(cols, cfg.ColumnMap, cfg.TagColumn)

	result := ingestion.SyncResult{}
	rowNum := 1 // header counted as row 0 for operator-facing error reports
	for {
		rowNum++
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, ingestion.SyncError{
				Reason: fmt.Sprintf("row %d: %v", rowNum, err),
			})
			result.AssetsRejected++
			continue
		}
		asset, perr := pick.toAsset(row, cfg.AssetType, rowNum)
		if perr != nil {
			result.Errors = append(result.Errors, ingestion.SyncError{
				ExternalID: pick.externalID(row),
				Reason:     fmt.Sprintf("row %d: %s", rowNum, perr.Error()),
			})
			result.AssetsRejected++
			continue
		}
		// NOTE: we do NOT call asset.Validate() here — Validate requires
		// OrgID/SourceID, which the writer fills from its binding. The
		// store-side UpsertKnowledgeAsset runs the full Validate after
		// binding. Row-level checks (blank title, malformed payload) are
		// performed by pick.toAsset above.
		//
		// Tags + payload normalisation happens at the writer layer
		// via the store's NormalizeTags call, but we do it here too so
		// the rejected-asset path sees the same shape as the accepted
		// path (operator-facing tag display matches what's stored).
		asset.Tags = assets.NormalizeTags(asset.Tags)
		if err := w.Write(ctx, asset); err != nil {
			// A writer error is a real failure — not a row-level
			// validation problem. Stop the sync and surface as a hard
			// error so the dispatcher updates source health to "error".
			return result, ingestion.WrapRecoverable(fmt.Errorf("csv: write row %d: %w", rowNum, err))
		}
		// We don't know from the writer return whether this was
		// create vs update — the dispatcher attributes both to
		// AssetsSeen. Detailed split lives in the store layer and is
		// exposed via Phase D metrics, not this counter.
		result.AssetsSeen++
	}
	_ = time.Now() // reserved: future fingerprint timestamp use
	return result, nil
}
