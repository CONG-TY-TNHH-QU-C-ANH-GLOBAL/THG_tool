package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
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
	wsURL, err := chromeBrowserWSPort(cdpPort)
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

// chromeBrowserWSPort fetches the CDP WebSocket URL from a running Chrome's debug endpoint.
func chromeBrowserWSPort(port int) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &info); err != nil || info.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("cannot read Chrome CDP endpoint on port %d", port)
	}
	return info.WebSocketDebuggerURL, nil
}
