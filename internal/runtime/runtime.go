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
	Timestamp        time.Time
	Reactions        int
	Comments         int
	Shares           int
}

// Runtime abstracts browser execution. The MVP uses MockRuntime.
// A real implementation would drive chromedp against a live container.
type Runtime interface {
	// FetchBatch fetches up to batchSize items from sourceURL starting at offset.
	// Returns an empty slice when exhausted.
	FetchBatch(ctx context.Context, sourceURL string, offset, batchSize int) ([]RawItem, error)
}
