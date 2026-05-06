package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/cdpclient"
)

// AttachedPool implements Browser by reusing a running Chrome via its CDP debug port.
// It does NOT launch or kill Chrome — it only opens/closes tabs in the existing instance.
// This is used when a workspace Chrome is already running for an account.
type AttachedPool struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	cdpPort     int
}

// NewAttachedPool connects to a running Chrome on cdpPort.
// Returns an error if Chrome is not reachable.
func NewAttachedPool(cdpPort int) (*AttachedPool, error) {
	wsURL, err := cdpclient.BrowserWSFromPort(cdpPort)
	if err != nil {
		return nil, fmt.Errorf("chrome not reachable on port %d: %w", cdpPort, err)
	}
	allocCtx, cancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	return &AttachedPool{
		allocCtx:    allocCtx,
		allocCancel: cancel,
		cdpPort:     cdpPort,
	}, nil
}

// Acquire creates a new browser tab in the attached Chrome.
func (p *AttachedPool) Acquire(timeout time.Duration) (*BrowserCtx, error) {
	ctx, cancel := context.WithTimeout(p.allocCtx, timeout)
	tabCtx, tabCancel := chromedp.NewContext(ctx)
	combinedCancel := func() { tabCancel(); cancel() }
	return &BrowserCtx{
		Ctx:    tabCtx,
		Cancel: combinedCancel,
		InUse:  true,
	}, nil
}

// Release closes the tab context (does not kill Chrome).
func (p *AttachedPool) Release(bCtx *BrowserCtx) {
	if bCtx != nil && bCtx.Cancel != nil {
		bCtx.Cancel()
	}
}

// Shutdown disconnects from Chrome (does not kill it — workspace Chrome keeps running).
func (p *AttachedPool) Shutdown() {
	p.allocCancel()
}

// ParentCtx returns the allocator context (lifetime of the CDP connection).
func (p *AttachedPool) ParentCtx() context.Context {
	return p.allocCtx
}
