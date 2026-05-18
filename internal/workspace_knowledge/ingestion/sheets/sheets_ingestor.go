// Package sheets is the Google Sheets ingestor stub.
//
// STATUS: stub. Returns a permanent error from Sync.
//
// Finishing in Phase B.3:
//   1. Use google.golang.org/api/sheets/v4 with a service-account
//      JWT carried in src.ConnectionConfig.service_account_json.
//   2. src.ConnectionConfig.sheet_id selects the spreadsheet;
//      src.ConnectionConfig.range_a1 (default "Sheet1!A:Z") narrows
//      the read.
//   3. The first row is the header; map columns into Asset fields
//      using the same ColumnMap conventions as the CSV ingestor.
//      Actually — share the implementation. csv/columns.go is
//      header-driven and storage-agnostic; pull it up into a
//      common package (workspace_knowledge/ingestion/tabular) and
//      let both csv and sheets consume it.
//   4. ExternalID = sheet_id + "!" + row_index. Stable across syncs.
//   5. Auth failures → ingestion.WrapPermanent and surface as
//      needs_auth via the dispatcher's health translation.
package sheets

import (
	"context"
	"errors"

	"github.com/thg/scraper/internal/workspace_knowledge/ingestion"
	"github.com/thg/scraper/internal/workspace_knowledge/sources"
)

type Ingestor struct{}

func New() *Ingestor                         { return &Ingestor{} }
func (i *Ingestor) Type() sources.SourceType { return sources.SourceGoogleSheets }

func (i *Ingestor) Sync(_ context.Context, _ *sources.Source, _ ingestion.AssetWriter) (ingestion.SyncResult, error) {
	return ingestion.SyncResult{}, ingestion.WrapPermanent(errors.New(
		"google sheets ingestor not yet implemented — Phase B.3",
	))
}
