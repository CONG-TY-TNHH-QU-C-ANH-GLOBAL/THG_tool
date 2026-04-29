package browser

import (
	"context"
	"time"
)

// Browser is the abstraction used by crawl handlers.
// Implementations attach to an existing workspace Chrome instead of launching
// a hidden scraper-owned browser.
type Browser interface {
	Acquire(timeout time.Duration) (*BrowserCtx, error)
	Release(ctx *BrowserCtx)
	Shutdown()
	ParentCtx() context.Context
}

// BrowserCtx wraps a chromedp tab context with metadata.
type BrowserCtx struct {
	Ctx    context.Context
	Cancel context.CancelFunc
	ID     int
	InUse  bool
}
