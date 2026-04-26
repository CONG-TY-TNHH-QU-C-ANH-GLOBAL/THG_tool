package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// chromeWSURL probes Chrome's CDP endpoint on cdpPort and returns the
// WebSocket debugger URL needed to attach a remote allocator.
func chromeWSURL(cdpPort int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", cdpPort)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("cdp probe %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var meta struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &meta); err != nil || meta.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("cdp: unexpected response from %s: %s", url, body)
	}
	return meta.WebSocketDebuggerURL, nil
}
