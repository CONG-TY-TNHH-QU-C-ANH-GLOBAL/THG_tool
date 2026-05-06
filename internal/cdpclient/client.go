package cdpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type Endpoint struct {
	BaseURL string
	WSHost  string
	Label   string
}

type TargetInfo struct {
	ID    string `json:"id"`
	WS    string `json:"webSocketDebuggerUrl"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func EndpointFromPort(port int) Endpoint {
	host := fmt.Sprintf("127.0.0.1:%d", port)
	return Endpoint{
		BaseURL: "http://" + host,
		WSHost:  host,
		Label:   host,
	}
}

func BrowserWSFromPort(port int) (string, error) {
	return BrowserWSFromEndpoint(EndpointFromPort(port))
}

func BrowserWSFromEndpoint(ep Endpoint) (string, error) {
	if ep.BaseURL == "" {
		return "", fmt.Errorf("chrome CDP endpoint is empty")
	}
	versionURL := strings.TrimRight(ep.BaseURL, "/") + "/json/version"
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(versionURL)
	if err != nil {
		return "", fmt.Errorf("chrome not ready: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &info); err != nil || info.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("cannot parse chrome debug endpoint %s", versionURL)
	}
	if u, err := url.Parse(info.WebSocketDebuggerURL); err == nil && u.Scheme != "" {
		if ep.WSHost != "" {
			u.Host = ep.WSHost
		} else if base, err := url.Parse(ep.BaseURL); err == nil {
			u.Host = base.Host
		}
		info.WebSocketDebuggerURL = u.String()
	}
	return info.WebSocketDebuggerURL, nil
}

func ContextForPort(port int, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	return ContextForEndpoint(EndpointFromPort(port), timeout)
}

func ContextForEndpoint(ep Endpoint, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	wsURL, err := BrowserWSFromEndpoint(ep)
	if err != nil {
		return nil, nil, err
	}
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)
	cancel := func() {
		timeoutCancel()
		ctxCancel()
		allocCancel()
	}
	return ctx, cancel, nil
}

func FetchTargets(port int) ([]TargetInfo, error) {
	if port <= 0 {
		return nil, fmt.Errorf("CDP port not available")
	}
	return FetchTargetsFromEndpoint(EndpointFromPort(port))
}

func FetchTargetsFromEndpoint(ep Endpoint) ([]TargetInfo, error) {
	if ep.BaseURL == "" {
		return nil, fmt.Errorf("CDP endpoint not available")
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	endpoints := []string{"json/list", "json"}
	deadline := time.Now().Add(8 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		for _, endpoint := range endpoints {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			targetsURL := strings.TrimRight(ep.BaseURL, "/") + "/" + endpoint
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, targetsURL, nil)
			resp, err := client.Do(req)
			if err != nil {
				cancel()
				lastErr = err
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			cancel()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				lastErr = fmt.Errorf("CDP /%s HTTP %d", endpoint, resp.StatusCode)
				continue
			}
			var targets []TargetInfo
			if err := json.Unmarshal(body, &targets); err != nil {
				lastErr = fmt.Errorf("CDP /%s parse failed: %w", endpoint, err)
				continue
			}
			return targets, nil
		}
		time.Sleep(350 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("CDP target list timed out")
	}
	return nil, lastErr
}
