package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// chromeWSURL probes Chrome's CDP endpoint on cdpPort and returns the
// WebSocket debugger URL needed to attach a remote allocator.
func chromeWSURL(cdpPort int) (string, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/version", cdpPort)
	resp, err := http.Get(endpoint) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("cdp probe %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var meta struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("cdp: unexpected response from %s: %s", endpoint, body)
	}

	wsURL := meta.WebSocketDebuggerURL
	if u, err := url.Parse(wsURL); err == nil {
		u.Host = fmt.Sprintf("127.0.0.1:%d", cdpPort)
		wsURL = u.String()
	}
	return wsURL, nil
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
