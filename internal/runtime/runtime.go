package runtime

import (
	"context"
	"time"
)

// RawItem is the unfiltered data unit returned by a browser runtime fetch.
type RawItem struct {
	ID               string
	Content          string
	AuthorName       string
	AuthorProfileURL string
	SourceURL        string
	// PostFBID / GroupFBID carry Facebook's stable identifiers when the
	// scraper can extract them. They drive write-time URL canonicalisation
	// in leadingest.repairPrimaryURL when SourceURL is a shell or empty.
	PostFBID  string
	GroupFBID string
	// URLRepairPath records which branch built SourceURL (anchor_clean,
	// synth_from_fbid, dropped_transient). Surfaces as `url:<path>` in
	// leadingest Outcome.Signals for Phase 1 telemetry.
	URLRepairPath string
	Timestamp     time.Time
	Reactions     int
	Comments      int
	Shares        int
}

// Runtime abstracts browser execution. The MVP uses MockRuntime.
// A real implementation would drive chromedp against a live container.
type Runtime interface {
	// FetchBatch fetches up to batchSize items from sourceURL starting at offset.
	// Returns an empty slice when exhausted.
	FetchBatch(ctx context.Context, sourceURL string, offset, batchSize int) ([]RawItem, error)
}
