package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	cdpnetwork "github.com/chromedp/cdproto/network"
	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var version = "dev"

const capabilitiesJSON = `{"native_companion":true,"browser_control":"user_device","screen_capture":true,"multi_profile":true,"extension_bridge":"optional"}`

type connectorConfig struct {
	ServerURL     string    `json:"server_url"`
	DeviceToken   string    `json:"device_token"`
	ConnectorID   int64     `json:"connector_id"`
	ConnectorName string    `json:"connector_name"`
	WSPath        string    `json:"ws_path"`
	APIBase       string    `json:"api_base"`
	PairedAt      time.Time `json:"paired_at"`
}

type pairResponse struct {
	DeviceToken string `json:"device_token"`
	Connector   struct {
		ID    int64  `json:"id"`
		OrgID int64  `json:"org_id"`
		Name  string `json:"name"`
	} `json:"connector"`
	WSPath  string `json:"ws_path"`
	APIBase string `json:"api_base"`
}

type chromeBridge struct {
	accountID   int64
	accountName string
	port        int
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
}

type chromeSnapshot struct {
	AccountID      int64
	AccountName    string
	CurrentURL     string
	FBUserID       string
	Status         string
	ScreenshotData string
}

type browserTarget struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	FBUserID    string `json:"fb_user_id"`
	Status      string `json:"status"`
}

type browserTargetsResponse struct {
	Targets []browserTarget `json:"targets"`
}

func main() {
	defaultServer := os.Getenv("THG_SERVER_URL")
	if defaultServer == "" {
		defaultServer = "https://sale.thgfulfill.com"
	}

	serverFlag := flag.String("server", defaultServer, "THG server URL")
	pairFlag := flag.String("pair", "", "one-time pairing code from the dashboard")
	resetFlag := flag.Bool("reset", false, "remove saved connector token and pair again")
	onceFlag := flag.Bool("once", false, "send one heartbeat then exit")
	noChromeFlag := flag.Bool("no-chrome", false, "only report connector heartbeat; do not open or inspect local Chrome")
	chromePortFlag := flag.Int("chrome-port", 9222, "local Chrome DevTools port")
	flag.Parse()

	serverURL := normalizeServerURL(*serverFlag)
	configPath := connectorConfigPath()

	fmt.Println("==================================================")
	fmt.Println("        THG LOCAL CONNECTOR")
	fmt.Println("==================================================")
	fmt.Println("Dashboard browser stream: this app runs isolated local Chrome profiles on this device.")
	fmt.Println("The THG Chrome Extension can verify your personal Chrome session, but dashboard")
	fmt.Println("streaming and multi-account automation run through this local runtime.")
	fmt.Println()
	fmt.Println("Server:", serverURL)
	fmt.Println("Config:", configPath)
	fmt.Println()

	if *resetFlag {
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			exitWithError("Could not reset connector config", err)
		}
		fmt.Println("Saved connector token removed. Pairing will start again.")
	}

	cfg, err := loadConnectorConfig(configPath)
	if err != nil {
		exitWithError("Could not read connector config", err)
	}
	if cfg.DeviceToken == "" {
		code := strings.TrimSpace(*pairFlag)
		if code == "" {
			code = promptLine("Enter pairing code from Browser workspace: ")
		}
		if code == "" {
			exitWithError("Pairing code is required", nil)
		}
		paired, err := pairConnector(serverURL, code)
		if err != nil {
			exitWithError("Pairing failed", err)
		}
		cfg = connectorConfig{
			ServerURL:     serverURL,
			DeviceToken:   paired.DeviceToken,
			ConnectorID:   paired.Connector.ID,
			ConnectorName: paired.Connector.Name,
			WSPath:        defaultString(paired.WSPath, "/ws/agent"),
			APIBase:       defaultString(paired.APIBase, "/api"),
			PairedAt:      time.Now().UTC(),
		}
		if err := saveConnectorConfig(configPath, cfg); err != nil {
			exitWithError("Could not save connector token", err)
		}
		fmt.Printf("Paired: %s (device #%d)\n", cfg.ConnectorName, cfg.ConnectorID)
		fmt.Println("The dashboard will use the saved device token from now on.")
		fmt.Println()
	} else if cfg.ServerURL != "" {
		serverURL = normalizeServerURL(cfg.ServerURL)
		fmt.Printf("Using saved connector: %s (device #%d)\n\n", cfg.ConnectorName, cfg.ConnectorID)
	}

	if err := sendHeartbeat(serverURL, cfg.DeviceToken, chromeSnapshot{Status: "connector_online"}); err != nil {
		if isDeviceTokenRejected(err) {
			exitWithError("Heartbeat failed", err)
		}
		fmt.Println("[warn] initial heartbeat failed:", err)
	}
	fmt.Println("Connector is online. You can return to the dashboard Browser tab.")
	if *onceFlag {
		return
	}
	if *noChromeFlag {
		runHeartbeatLoop(serverURL, cfg.DeviceToken, nil)
		return
	}
	runConnectorLoop(serverURL, cfg.DeviceToken, *chromePortFlag)
}

func normalizeServerURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "https://sale.thgfulfill.com"
	}
	return strings.TrimRight(value, "/")
}

func promptLine(label string) string {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func connectorConfigPath() string {
	root, err := os.UserConfigDir()
	if err != nil || root == "" {
		root = "."
	}
	return filepath.Join(root, "THG Local Connector", "config.json")
}

func loadConnectorConfig(path string) (connectorConfig, error) {
	var cfg connectorConfig
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func saveConnectorConfig(path string, cfg connectorConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func pairConnector(serverURL, code string) (*pairResponse, error) {
	hostname, _ := os.Hostname()
	body, _ := json.Marshal(map[string]any{
		"code":              code,
		"hostname":          hostname,
		"os":                runtime.GOOS + "/" + runtime.GOARCH,
		"version":           version,
		"capabilities_json": capabilitiesJSON,
		"stream_status":     "pairing",
	})
	resp, err := http.Post(serverURL+"/api/connectors/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out pairResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.DeviceToken) == "" {
		return nil, fmt.Errorf("server did not return a device token")
	}
	return &out, nil
}

func startChromeBridge(port int) *chromeBridge {
	return startChromeBridgeForTarget(browserTarget{AccountID: 0, AccountName: "Default Facebook"}, port)
}

func startChromeBridgeForTarget(target browserTarget, port int) *chromeBridge {
	if port <= 0 {
		port = 9222
	}
	devtoolsURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	wsURL, err := chromeWebSocketURL(devtoolsURL)
	if err != nil {
		if launchErr := launchChrome(port, chromeUserDataDir(target.AccountID)); launchErr != nil {
			return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: fmt.Errorf("%v; launch chrome: %w", err, launchErr)}
		}
		wsURL, err = waitChromeWebSocketURL(devtoolsURL, 15*time.Second)
		if err != nil {
			return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: err}
		}
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, cancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.facebook.com"),
		chromedp.Sleep(2*time.Second),
	); err != nil {
		cancel()
		allocCancel()
		return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: err}
	}
	return &chromeBridge{
		accountID:   target.AccountID,
		accountName: target.AccountName,
		port:        port,
		ctx:         ctx,
		cancel: func() {
			cancel()
			allocCancel()
		},
	}
}

func chromeWebSocketURL(devtoolsURL string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(devtoolsURL + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Chrome DevTools returned %d", resp.StatusCode)
	}
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.WebSocketDebuggerURL) == "" {
		return "", fmt.Errorf("Chrome DevTools URL is empty")
	}
	return payload.WebSocketDebuggerURL, nil
}

func waitChromeWebSocketURL(devtoolsURL string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		wsURL, err := chromeWebSocketURL(devtoolsURL)
		if err == nil {
			return wsURL, nil
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Chrome DevTools did not become ready")
	}
	return "", lastErr
}

func launchChrome(port int, userDataDir string) error {
	chromePath, err := findChromePath()
	if err != nil {
		return err
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=1365,900",
		"https://www.facebook.com",
	}
	if strings.TrimSpace(os.Getenv("THG_CHROME_VISIBLE")) != "1" {
		args = append([]string{"--window-position=-32000,-32000"}, args...)
	}
	if userDataDir != "" {
		if err := os.MkdirAll(userDataDir, 0700); err != nil {
			return err
		}
		args = append([]string{fmt.Sprintf("--user-data-dir=%s", userDataDir)}, args...)
	}
	cmd := exec.Command(chromePath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Start()
}

func findChromePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CHROME_PATH")); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	candidates := chromePathCandidates()
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, string(os.PathSeparator)) {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("Google Chrome was not found")
}

func chromePathCandidates() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
			"chrome.exe",
		}
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"google-chrome",
			"chromium",
		}
	default:
		return []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"}
	}
}

func chromeUserDataDir(accountID int64) string {
	if dir := strings.TrimSpace(os.Getenv("THG_CHROME_USER_DATA_DIR")); dir != "" {
		return dir
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	name := "account-" + strconv.FormatInt(accountID, 10)
	if accountID <= 0 {
		name = "default"
	}
	return filepath.Join(root, "THG Local Connector", "chrome-profiles", name)
}

func snapshotChrome(bridge *chromeBridge) chromeSnapshot {
	if bridge == nil {
		return chromeSnapshot{Status: "connector_online"}
	}
	if bridge.err != nil || bridge.ctx == nil {
		return chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, Status: "chrome_not_connected"}
	}
	var href string
	var fbUserID string
	var screenshot []byte
	err := chromedp.Run(bridge.ctx,
		chromedp.Location(&href),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := cdpnetwork.GetCookies().WithURLs([]string{
				"https://www.facebook.com",
				"https://facebook.com",
			}).Do(ctx)
			if err != nil {
				return err
			}
			for _, ck := range cookies {
				if ck.Name == "c_user" && ck.Value != "" {
					fbUserID = ck.Value
					break
				}
			}
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			data, err := cdppage.CaptureScreenshot().
				WithFormat(cdppage.CaptureScreenshotFormatJpeg).
				WithQuality(40).
				Do(ctx)
			if err == nil && len(data) > 0 {
				screenshot = data
			}
			return nil
		}),
	)
	if err != nil {
		return chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, Status: "chrome_not_connected"}
	}
	status := "facebook_login_required"
	lowerURL := strings.ToLower(href)
	if strings.Contains(lowerURL, "checkpoint") || strings.Contains(lowerURL, "two_step") {
		status = "facebook_human_required"
	}
	if fbUserID != "" {
		status = "facebook_logged_in"
	}
	out := chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, CurrentURL: href, FBUserID: fbUserID, Status: status}
	if len(screenshot) > 0 {
		out.ScreenshotData = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(screenshot)
	}
	return out
}

func sendHeartbeat(serverURL, token string, snap chromeSnapshot) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token; run with --reset and pair again")
	}
	hostname, _ := os.Hostname()
	body, _ := json.Marshal(map[string]any{
		"hostname":          hostname,
		"os":                runtime.GOOS + "/" + runtime.GOARCH,
		"version":           version,
		"kind":              "desktop_connector",
		"transport":         "local_chrome",
		"capabilities_json": capabilitiesJSON,
		"current_url":       snap.CurrentURL,
		"fb_user_id":        snap.FBUserID,
		"stream_status":     defaultString(snap.Status, "connector_online"),
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/connectors/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("device token was rejected; run with --reset and pair again")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func sendChromeStatus(serverURL, token string, snap chromeSnapshot) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token")
	}
	body, _ := json.Marshal(map[string]any{
		"account_id":    snap.AccountID,
		"current_url":   snap.CurrentURL,
		"fb_user_id":    snap.FBUserID,
		"stream_status": defaultString(snap.Status, "chrome_not_connected"),
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/connectors/chrome-status", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("device token was rejected; run with --reset and pair again")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func sendScreenshot(serverURL, token string, snap chromeSnapshot) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing saved device token")
	}
	if snap.AccountID <= 0 || snap.ScreenshotData == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{
		"account_id":    snap.AccountID,
		"image_data":    snap.ScreenshotData,
		"current_url":   snap.CurrentURL,
		"fb_user_id":    snap.FBUserID,
		"stream_status": defaultString(snap.Status, "connector_online"),
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/connectors/screenshot", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("device token was rejected; run with --reset and pair again")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func fetchBrowserTargets(serverURL, token string) ([]browserTarget, error) {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/connectors/browser-targets", nil)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("device token was rejected; run with --reset and pair again")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out browserTargetsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Targets, nil
}

func probeExistingChromeStatus(port int) chromeSnapshot {
	if port <= 0 {
		port = 9222
	}
	devtoolsURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	targets, err := chromeTargets(devtoolsURL)
	if err != nil {
		return chromeSnapshot{Status: "chrome_not_connected"}
	}
	status := "chrome_connected"
	var currentURL string
	for _, target := range targets {
		if target.Type != "page" {
			continue
		}
		if currentURL == "" {
			currentURL = target.URL
		}
		if strings.Contains(strings.ToLower(target.URL), "facebook.com") {
			currentURL = target.URL
			status = "facebook_login_required"
			break
		}
	}
	return chromeSnapshot{CurrentURL: currentURL, Status: status}
}

type chromeTargetInfo struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func chromeTargets(devtoolsURL string) ([]chromeTargetInfo, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(devtoolsURL + "/json/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Chrome DevTools target list returned %d", resp.StatusCode)
	}
	var out []chromeTargetInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func runConnectorLoop(serverURL, token string, basePort int) {
	if basePort <= 0 {
		basePort = 9222
	}
	bridges := map[int64]*chromeBridge{}
	ticker := time.NewTicker(frameInterval())
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	defer func() {
		for _, bridge := range bridges {
			if bridge != nil && bridge.cancel != nil {
				bridge.cancel()
			}
		}
	}()

	syncTargets := func() bool {
		targets, err := fetchBrowserTargets(serverURL, token)
		if err != nil {
			if isDeviceTokenRejected(err) {
				fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
				return false
			}
			fmt.Println("[warn] target sync failed:", err)
			return true
		}
		want := map[int64]browserTarget{}
		for _, target := range targets {
			if target.AccountID <= 0 {
				continue
			}
			want[target.AccountID] = target
			if _, ok := bridges[target.AccountID]; ok {
				continue
			}
			port := localChromePort(basePort, target.AccountID)
			fmt.Printf("[Chrome] Opening %s on local port %d\n", target.AccountName, port)
			bridge := startChromeBridgeForTarget(target, port)
			if bridge.err != nil {
				fmt.Printf("[Chrome] %s not ready: %v\n", target.AccountName, bridge.err)
			}
			bridges[target.AccountID] = bridge
		}
		for accountID, bridge := range bridges {
			if _, ok := want[accountID]; ok {
				continue
			}
			if bridge != nil && bridge.cancel != nil {
				bridge.cancel()
			}
			delete(bridges, accountID)
		}
		return true
	}

	sendFrames := func() bool {
		best := probeExistingChromeStatus(basePort)
		if len(bridges) == 0 {
			if err := sendChromeStatus(serverURL, token, best); err != nil {
				if isDeviceTokenRejected(err) {
					fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
					return false
				}
				fmt.Println("[warn] chrome status failed:", err)
			}
		}
		for _, bridge := range bridges {
			snap := snapshotChrome(bridge)
			if best.AccountID == 0 || best.Status == "chrome_not_connected" || snap.Status == "facebook_logged_in" {
				best = snap
			}
			if err := sendChromeStatus(serverURL, token, snap); err != nil {
				if isDeviceTokenRejected(err) {
					fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
					return false
				}
				fmt.Printf("[warn] chrome status failed for account %d: %v\n", snap.AccountID, err)
			}
			if err := sendScreenshot(serverURL, token, snap); err != nil {
				if isDeviceTokenRejected(err) {
					fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
					return false
				}
				fmt.Printf("[warn] screenshot failed for account %d: %v\n", snap.AccountID, err)
			}
		}
		if err := sendHeartbeat(serverURL, token, best); err != nil {
			if isDeviceTokenRejected(err) {
				fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
				return false
			}
			fmt.Println("[warn] heartbeat failed:", err)
			return true
		}
		fmt.Printf("heartbeat ok %s - %d Chrome profile(s) - %s\n", time.Now().Format("15:04:05"), len(bridges), connectorConsoleStatus(best))
		return true
	}

	if !syncTargets() || !sendFrames() {
		return
	}
	for {
		select {
		case <-ticker.C:
			if !syncTargets() || !sendFrames() {
				return
			}
		case <-stop:
			fmt.Println()
			fmt.Println("Connector stopped.")
			return
		}
	}
}

func localChromePort(basePort int, accountID int64) int {
	if basePort <= 0 {
		basePort = 9222
	}
	offset := int(accountID % 10000)
	if offset < 0 {
		offset = -offset
	}
	port := basePort + offset
	if port > 65000 {
		port = 20000 + offset
	}
	return port
}

func frameInterval() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_FRAME_INTERVAL_SECONDS")))
	if seconds <= 0 {
		seconds = 2
	}
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func runHeartbeatLoop(serverURL, token string, bridge *chromeBridge) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	for {
		select {
		case <-ticker.C:
			snap := snapshotChrome(bridge)
			if err := sendHeartbeat(serverURL, token, snap); err != nil {
				if isDeviceTokenRejected(err) {
					fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
					return
				}
				fmt.Println("[warn] heartbeat failed:", err)
				continue
			}
			fmt.Printf("heartbeat ok %s - %s\n", time.Now().Format("15:04:05"), connectorConsoleStatus(snap))
		case <-stop:
			fmt.Println()
			fmt.Println("Connector stopped.")
			return
		}
	}
}

func isDeviceTokenRejected(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "device token was rejected")
}

func connectorConsoleStatus(snap chromeSnapshot) string {
	switch snap.Status {
	case "facebook_logged_in":
		if snap.FBUserID != "" {
			return "Facebook connected: " + snap.FBUserID
		}
		return "Facebook connected"
	case "facebook_human_required":
		return "Facebook needs human verification"
	case "facebook_login_required":
		return "Facebook tab is open but not logged in"
	case "chrome_not_connected":
		return "Chrome is not connected"
	default:
		return "connector online"
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func exitWithError(message string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", message, err)
	} else {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(1)
}
