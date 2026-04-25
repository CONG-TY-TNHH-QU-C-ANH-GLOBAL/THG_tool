package browser

import (
	"context"
	"time"
)

// Browser is the abstraction used by all scrapers.
// Both Pool (launches its own Chrome) and AttachedPool (reuses a running Chrome) implement it.
type Browser interface {
	Acquire(timeout time.Duration) (*BrowserCtx, error)
	Release(ctx *BrowserCtx)
	Shutdown()
	ParentCtx() context.Context
}
