package runtime

import (
	"context"
	"fmt"
	"strings"

	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/thg/scraper/internal/cdpclient"
)

// chromeWSURL probes Chrome's CDP endpoint on cdpPort and returns the
// WebSocket debugger URL needed to attach a remote allocator.
func chromeWSURL(cdpPort int) (string, error) {
	return cdpclient.BrowserWSFromPort(cdpPort)
}

func visiblePageTargetID(ctx context.Context) (cdptarget.ID, error) {
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return "", fmt.Errorf("list cdp targets: %w", err)
	}

	var fallback cdptarget.ID
	for _, t := range targets {
		if t == nil || t.Type != "page" || strings.HasPrefix(t.URL, "devtools://") {
			continue
		}
		if strings.Contains(t.URL, "facebook.com") {
			return t.TargetID, nil
		}
		if fallback == "" {
			fallback = t.TargetID
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no visible page target")
}
