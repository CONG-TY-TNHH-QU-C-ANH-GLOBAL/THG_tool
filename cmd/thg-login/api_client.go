package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

func sendHeartbeat(serverURL, token string, snap chromeSnapshot) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token; run with --reset and pair again")
	}
	body := map[string]any{
		"hostname":          hostname(),
		"os":                runtime.GOOS + "/" + runtime.GOARCH,
		"version":           version,
		"kind":              "desktop_connector",
		"transport":         "local_chrome",
		"account_id":        snap.AccountID,
		"capabilities_json": capabilitiesJSON,
		"current_url":       snap.CurrentURL,
		"fb_user_id":        snap.FBUserID,
		"fb_display_name":   snap.FBDisplayName,
		"fb_username":       snap.FBUsername,
		"fb_profile_url":    snap.FBProfileURL,
		"login_email":       snap.LoginEmail,
		"stream_status":     strings.TrimSpace(snap.Status),
		"chrome_error":      snap.ChromeError,
	}
	return postAgentJSON(serverURL+"/api/connectors/heartbeat", token, body, 15*time.Second, nil)
}

func sendChromeStatus(serverURL, token string, snap chromeSnapshot) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token")
	}
	body := map[string]any{
		"account_id":      snap.AccountID,
		"current_url":     snap.CurrentURL,
		"fb_user_id":      snap.FBUserID,
		"fb_display_name": snap.FBDisplayName,
		"fb_username":     snap.FBUsername,
		"fb_profile_url":  snap.FBProfileURL,
		"login_email":     snap.LoginEmail,
		"stream_status":   defaultString(snap.Status, streamStatusChromeNotConnected),
		"chrome_error":    snap.ChromeError,
	}
	return postAgentJSON(serverURL+"/api/connectors/chrome-status", token, body, 15*time.Second, nil)
}

func sendScreenshot(serverURL, token string, snap chromeSnapshot) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token")
	}
	if snap.AccountID <= 0 || snap.ScreenshotData == "" {
		return nil
	}
	body := map[string]any{
		"account_id":      snap.AccountID,
		"image_data":      snap.ScreenshotData,
		"current_url":     snap.CurrentURL,
		"fb_user_id":      snap.FBUserID,
		"fb_display_name": snap.FBDisplayName,
		"fb_username":     snap.FBUsername,
		"fb_profile_url":  snap.FBProfileURL,
		"login_email":     snap.LoginEmail,
		"stream_status":   defaultString(snap.Status, streamStatusConnectorOnline),
		"chrome_error":    snap.ChromeError,
	}
	return postAgentJSON(serverURL+"/api/connectors/screenshot", token, body, 20*time.Second, nil)
}

func fetchBrowserTargets(serverURL, token string) (browserTargetsResponse, error) {
	var out browserTargetsResponse
	err := getAgentJSON(serverURL+"/api/connectors/browser-targets", token, 15*time.Second, &out)
	return out, err
}

func fetchConnectorCommands(serverURL, token string) ([]connectorCommand, error) {
	var out connectorCommandsResponse
	if err := getAgentJSON(serverURL+"/api/connectors/commands?limit=50", token, 10*time.Second, &out); err != nil {
		return nil, err
	}
	return out.Commands, nil
}

func completeConnectorCommand(serverURL, token string, id int64, errorText string) error {
	body := map[string]any{"error": errorText}
	return postAgentJSON(fmt.Sprintf("%s/api/connectors/commands/%d/done", serverURL, id), token, body, 10*time.Second, nil)
}

func fetchApprovedOutbox(serverURL, token string) ([]outboundMessage, error) {
	var out outboxResponse
	if err := getAgentJSON(serverURL+"/api/connectors/outbox?limit=5", token, 10*time.Second, &out); err != nil {
		return nil, err
	}
	return out.Messages, nil
}

func completeOutboxMessage(serverURL, token string, id int64, success bool, errorText string) error {
	path := "failed"
	if success {
		path = "sent"
	}
	body := map[string]any{"error": strings.TrimSpace(errorText)}
	return postAgentJSON(fmt.Sprintf("%s/api/connectors/outbox/%d/%s", serverURL, id, path), token, body, 10*time.Second, nil)
}

func getAgentJSON(rawURL, token string, timeout time.Duration, out any) error {
	req, err := newAgentRequest(http.MethodGet, rawURL, token, nil)
	if err != nil {
		return err
	}
	return doAgentJSON(req, timeout, out)
}

func postAgentJSON(rawURL, token string, body any, timeout time.Duration, out any) error {
	req, err := newAgentRequest(http.MethodPost, rawURL, token, body)
	if err != nil {
		return err
	}
	return doAgentJSON(req, timeout, out)
}

func newAgentRequest(method, rawURL, token string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname())
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)
	return req, nil
}

func doAgentJSON(req *http.Request, timeout time.Duration, out any) error {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkAgentResponse(resp); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func checkAgentResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	data, _ := io.ReadAll(resp.Body)
	detail := strings.TrimSpace(string(data))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if detail == "" {
			detail = "dashboard rejected the saved device token"
		}
		return fmt.Errorf("device token was rejected (%d): %s; run with --reset and pair again", resp.StatusCode, detail)
	}
	if detail == "" {
		detail = http.StatusText(resp.StatusCode)
	}
	return fmt.Errorf("server returned %d: %s", resp.StatusCode, detail)
}

func hostname() string {
	name, _ := os.Hostname()
	return name
}
