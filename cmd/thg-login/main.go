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
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	cdpinput "github.com/chromedp/cdproto/input"
	cdpnetwork "github.com/chromedp/cdproto/network"
	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

var version = "dev"

const capabilitiesJSON = `{"native_companion":true,"browser_control":"user_device","screen_capture":true,"multi_profile":true,"dashboard_stream":true,"input_relay":true,"local_login_first":true,"hide_local_after_login":true}`

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
	accountID         int64
	accountName       string
	port              int
	pid               int
	ctx               context.Context
	cancel            context.CancelFunc
	err               error
	loginIdentifier   string
	loginCaptureLog   string
	windowHidden      bool
	windowWarned      bool
	lastWindowPosture time.Time
	lastLoginRecovery time.Time
}

type chromeSnapshot struct {
	AccountID      int64
	AccountName    string
	CurrentURL     string
	FBUserID       string
	LoginEmail     string
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

type connectorCommand struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	Type        string `json:"type"`
	PayloadJSON string `json:"payload_json"`
}

type connectorCommandsResponse struct {
	Commands []connectorCommand `json:"commands"`
}

type localCrawlTask struct {
	TaskID    string            `json:"task_id"`
	OrgID     int64             `json:"org_id"`
	AccountID int64             `json:"account_id"`
	Intent    string            `json:"intent"`
	Keywords  []string          `json:"keywords"`
	CrawlPlan localCrawlPlan    `json:"crawl_plan"`
	Filters   localCrawlFilters `json:"filters"`
}

type localCrawlPlan struct {
	Sources   []localCrawlSource `json:"sources"`
	MaxItems  int                `json:"max_items"`
	BatchSize int                `json:"batch_size"`
}

type localCrawlSource struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

type localCrawlFilters struct {
	Keywords []string `json:"keywords"`
}

type localCrawlItem struct {
	ID               string `json:"id"`
	SourceURL        string `json:"source_url"`
	AuthorProfileURL string `json:"author_profile_url"`
	AuthorName       string `json:"author_name"`
	Content          string `json:"content"`
	Reactions        int    `json:"reactions"`
	Comments         int    `json:"comments"`
	Shares           int    `json:"shares"`
}

type localCrawlResult struct {
	TaskID    string           `json:"task_id"`
	Intent    string           `json:"intent"`
	AccountID int64            `json:"account_id"`
	Status    string           `json:"status"`
	Error     string           `json:"error,omitempty"`
	Keywords  []string         `json:"keywords"`
	Items     []localCrawlItem `json:"items"`
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
	fmt.Println("Local login first: THG opens a Chrome profile on this device for Facebook login/checkpoint.")
	fmt.Println("After Facebook is ready, the Browser dashboard observes and controls automation through this Runtime.")
	fmt.Println("When Facebook login succeeds, the local Chrome window is moved away so the dashboard becomes the main workspace.")
	fmt.Println("THG does not ask for your Facebook password or upload your Facebook password.")
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
		cfg = mustPairAndSave(serverURL, configPath, strings.TrimSpace(*pairFlag), "Enter pairing code from Browser workspace: ")
	} else if cfg.ServerURL != "" {
		serverURL = normalizeServerURL(cfg.ServerURL)
		fmt.Printf("Using saved connector: %s (device #%d)\n\n", cfg.ConnectorName, cfg.ConnectorID)
	}

	if err := sendHeartbeat(serverURL, cfg.DeviceToken, chromeSnapshot{Status: "connector_online"}); err != nil {
		if isDeviceTokenRejected(err) {
			fmt.Println("Saved device token was rejected by the server.")
			fmt.Println("This usually means the device was disconnected from the dashboard or the workspace was paired again.")
			if removeErr := os.Remove(configPath); removeErr != nil && !os.IsNotExist(removeErr) {
				exitWithError("Could not reset rejected connector config", removeErr)
			}
			fmt.Println("Old connector config removed. Create a new pairing code in the Browser dashboard.")
			cfg = mustPairAndSave(serverURL, configPath, strings.TrimSpace(*pairFlag), "Enter new pairing code: ")
			if err := sendHeartbeat(serverURL, cfg.DeviceToken, chromeSnapshot{Status: "connector_online"}); err != nil {
				exitWithError("Heartbeat failed after re-pairing", err)
			}
		} else {
			fmt.Println("[warn] initial heartbeat failed:", err)
		}
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

func mustPairAndSave(serverURL, configPath, code, prompt string) connectorConfig {
	code = strings.TrimSpace(code)
	if code == "" {
		code = promptLine(prompt)
	}
	if code == "" {
		exitWithError("Pairing code is required", nil)
	}
	paired, err := pairConnector(serverURL, code)
	if err != nil {
		exitWithError("Pairing failed", err)
	}
	cfg := connectorConfig{
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
	return cfg
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
	chromePID := findLocalChromeProcessID(port)
	wsURL, err := chromeWebSocketURL(devtoolsURL)
	if err != nil {
		pid, launchErr := launchChrome(port, chromeUserDataDir(target.AccountID))
		if launchErr != nil {
			return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: fmt.Errorf("%v; launch chrome: %w", err, launchErr)}
		}
		chromePID = pid
		wsURL, err = waitChromeWebSocketURL(devtoolsURL, 15*time.Second)
		if err != nil {
			return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: err}
		}
	}
	if pid := findLocalChromeProcessID(port); pid > 0 {
		chromePID = pid
	}
	if chromePID == 0 {
		chromePID = findLocalChromeProcessID(port)
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	ctx, cancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(ctx,
		installFacebookLoginCaptureOnNewDocument(),
		chromedp.Navigate("https://www.facebook.com"),
		chromedp.Sleep(2*time.Second),
		installFacebookLoginCapture(),
	); err != nil {
		cancel()
		allocCancel()
		return &chromeBridge{accountID: target.AccountID, accountName: target.AccountName, port: port, err: err}
	}
	return &chromeBridge{
		accountID:   target.AccountID,
		accountName: target.AccountName,
		port:        port,
		pid:         chromePID,
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

func launchChrome(port int, userDataDir string) (int, error) {
	chromePath, err := findChromePath()
	if err != nil {
		return 0, err
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--no-first-run",
		"--no-default-browser-check",
		"--force-device-scale-factor=1",
		"--high-dpi-support=1",
		"--window-size=1365,900",
		"https://www.facebook.com",
	}
	if shouldHideChromeWindow() {
		args = append([]string{"--window-position=-32000,-32000"}, args...)
	}
	if userDataDir != "" {
		if err := os.MkdirAll(userDataDir, 0700); err != nil {
			return 0, err
		}
		args = append([]string{fmt.Sprintf("--user-data-dir=%s", userDataDir)}, args...)
	}
	cmd := exec.Command(chromePath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	if cmd.Process == nil {
		return 0, nil
	}
	return cmd.Process.Pid, nil
}

func shouldHideChromeWindow() bool {
	visible := strings.TrimSpace(strings.ToLower(os.Getenv("THG_CHROME_VISIBLE")))
	headlessStream := strings.TrimSpace(strings.ToLower(os.Getenv("THG_CHROME_HEADLESS_STREAM")))
	return visible == "0" || visible == "false" || visible == "no" || visible == "off" ||
		headlessStream == "1" || headlessStream == "true" || headlessStream == "yes" || headlessStream == "on"
}

func keepLocalChromeVisibleAfterLogin() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("THG_KEEP_CHROME_VISIBLE_AFTER_LOGIN")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func hideChromeWindowAfterLogin(ctx context.Context, pid int) error {
	var failures []string
	hidden := false
	windowID, _, err := cdpbrowser.GetWindowForTarget().Do(ctx)
	if err == nil {
		if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateMinimized}).Do(ctx); err == nil {
			hidden = true
		} else {
			failures = append(failures, "cdp minimize: "+err.Error())
		}
		_ = cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateNormal}).Do(ctx)
		if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{
			Left:   -32000,
			Top:    -32000,
			Width:  1365,
			Height: 900,
		}).Do(ctx); err == nil {
			hidden = true
		} else {
			failures = append(failures, "cdp offscreen: "+err.Error())
		}
	} else {
		failures = append(failures, "cdp window: "+err.Error())
	}
	if err := hideLocalChromeProcessWindow(pid); err == nil {
		hidden = true
	} else if pid > 0 {
		failures = append(failures, "native hide: "+err.Error())
	}
	if hidden {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(failures, "; "))
}

func showChromeWindowForLogin(ctx context.Context, pid int) error {
	var failures []string
	shown := false
	if err := showLocalChromeProcessWindow(pid); err == nil {
		shown = true
	} else if pid > 0 {
		failures = append(failures, "native show: "+err.Error())
	}
	windowID, _, err := cdpbrowser.GetWindowForTarget().Do(ctx)
	if err != nil {
		if shown {
			return nil
		}
		return err
	}
	if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{WindowState: cdpbrowser.WindowStateNormal}).Do(ctx); err == nil {
		shown = true
	} else {
		failures = append(failures, "cdp normal: "+err.Error())
	}
	if err := cdpbrowser.SetWindowBounds(windowID, &cdpbrowser.Bounds{
		Left:   80,
		Top:    60,
		Width:  1365,
		Height: 900,
	}).Do(ctx); err == nil {
		shown = true
	} else {
		failures = append(failures, "cdp bounds: "+err.Error())
	}
	if shown {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(failures, "; "))
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
	var loginIdentifier string
	var loginFormVisible bool
	var screenshot []byte
	err := chromedp.Run(bridge.ctx,
		readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible),
	)
	if err != nil {
		return chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, Status: "chrome_not_connected"}
	}
	if loginIdentifier != "" {
		bridge.loginIdentifier = loginIdentifier
		logCapturedLoginIdentifier(bridge, loginIdentifier)
	}
	lowerURL := strings.ToLower(href)
	humanRequired := isFacebookHumanRequiredURL(lowerURL)
	if fbUserID != "" && loginFormVisible && !humanRequired && time.Since(bridge.lastLoginRecovery) > 5*time.Second {
		bridge.lastLoginRecovery = time.Now()
		fmt.Printf("[Chrome] %s has Facebook cookies but still shows login form. Reloading Facebook feed for dashboard stream.\n", bridge.accountName)
		_ = chromedp.Run(bridge.ctx,
			chromedp.Navigate("https://www.facebook.com/"),
			chromedp.Sleep(2*time.Second),
			readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible),
		)
		if loginIdentifier != "" {
			bridge.loginIdentifier = loginIdentifier
			logCapturedLoginIdentifier(bridge, loginIdentifier)
		}
		lowerURL = strings.ToLower(href)
		humanRequired = isFacebookHumanRequiredURL(lowerURL)
	}
	err = chromedp.Run(bridge.ctx,
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
	if humanRequired {
		status = "facebook_human_required"
	}
	if fbUserID != "" && !loginFormVisible && !humanRequired {
		status = "facebook_logged_in"
	}
	updateChromeWindowPosture(bridge, status)
	loginEmail := ""
	if status == "facebook_logged_in" {
		loginEmail = normalizeEmailCandidate(bridge.loginIdentifier)
	}
	out := chromeSnapshot{AccountID: bridge.accountID, AccountName: bridge.accountName, CurrentURL: href, FBUserID: fbUserID, LoginEmail: loginEmail, Status: status}
	if len(screenshot) > 0 {
		out.ScreenshotData = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(screenshot)
	}
	return out
}

func readFacebookPageState(href, fbUserID, loginIdentifier *string, loginFormVisible *bool) chromedp.Action {
	return chromedp.Tasks{
		chromedp.Location(href),
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := cdpnetwork.GetCookies().WithURLs([]string{
				"https://www.facebook.com",
				"https://facebook.com",
			}).Do(ctx)
			if err != nil {
				return err
			}
			*fbUserID = ""
			for _, ck := range cookies {
				if ck.Name == "c_user" && ck.Value != "" {
					*fbUserID = ck.Value
					break
				}
			}
			return nil
		}),
		installFacebookLoginCapture(),
		chromedp.Evaluate(`(() => {
			const email = document.querySelector('input[name="email"], input#email');
			const pass = document.querySelector('input[name="pass"], input#pass');
			const loginButton = document.querySelector('button[name="login"], input[name="login"]');
			const loginForm = document.querySelector('form[action*="login"], form[action*="/login/"]');
			return Boolean((email && pass) || (loginForm && loginButton));
		})()`, loginFormVisible),
		chromedp.Evaluate(facebookLoginIdentifierScript(), loginIdentifier),
	}
}

func installFacebookLoginCaptureOnNewDocument() chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := cdppage.AddScriptToEvaluateOnNewDocument(facebookLoginCaptureSource()).Do(ctx)
		return err
	})
}

func installFacebookLoginCapture() chromedp.Action {
	return chromedp.Evaluate(facebookLoginCaptureSource(), nil)
}

func facebookLoginCaptureSource() string {
	return `(() => {
		try {
			const key = "__thg_last_facebook_login_identifier";
			const prop = "__thgLastFacebookLoginIdentifier";
			const selectors = [
				'input[name="email"]',
				'input#email',
				'input[autocomplete="username"]',
				'input[type="email"]',
				'input[type="text"][name*="email" i]',
				'input[type="text"][autocomplete="username"]'
			];
			const readField = () => selectors.map((selector) => document.querySelector(selector)).find(Boolean);
			const save = (value) => {
				value = String(value || "").trim();
				if (!value) return "";
				value = value.slice(0, 320);
				window[prop] = value;
				try { window.localStorage.setItem(key, value); } catch (_) {}
				try { window.sessionStorage.setItem(key, value); } catch (_) {}
				return value;
			};
			const remember = () => {
				const field = readField();
				if (!field) return "";
				return save(field.value || field.getAttribute("value") || "");
			};
			const bindField = () => {
				const field = readField();
				if (!field || field.dataset.thgLoginCaptureBound) return Boolean(field);
				field.dataset.thgLoginCaptureBound = "1";
				["input", "change", "keyup", "keydown", "blur", "focusout"].forEach((eventName) => {
					field.addEventListener(eventName, remember, true);
				});
				remember();
				return true;
			};
			const bindDocument = () => {
				if (window.__thgFacebookLoginDocumentBound) return;
				window.__thgFacebookLoginDocumentBound = true;
				["submit", "click", "keydown", "beforeunload", "pagehide"].forEach((eventName) => {
					document.addEventListener(eventName, remember, true);
				});
				const observer = new MutationObserver(() => bindField());
				observer.observe(document.documentElement || document, { childList: true, subtree: true, attributes: true, attributeFilter: ["value"] });
			};
			bindDocument();
			bindField();
			return Boolean(window[prop] || (() => {
				try {
					return window.localStorage.getItem(key) || window.sessionStorage.getItem(key) || "";
				} catch (_) {
					return "";
				}
			})());
		} catch (_) {
			return false;
		}
	})()`
}

func facebookLoginIdentifierScript() string {
	return `(() => {
		try {
			const key = "__thg_last_facebook_login_identifier";
			const prop = "__thgLastFacebookLoginIdentifier";
			const fromWindow = String(window[prop] || "").trim();
			if (fromWindow) return fromWindow.slice(0, 320);
			let stored = "";
			try { stored = String(window.localStorage.getItem(key) || "").trim(); } catch (_) {}
			if (!stored) {
				try { stored = String(window.sessionStorage.getItem(key) || "").trim(); } catch (_) {}
			}
			if (stored) return stored.slice(0, 320);
			const selectors = [
				'input[name="email"]',
				'input#email',
				'input[autocomplete="username"]',
				'input[type="email"]',
				'input[type="text"][name*="email" i]',
				'input[type="text"][autocomplete="username"]'
			];
			const field = selectors.map((selector) => document.querySelector(selector)).find(Boolean);
			if (!field) return "";
			const value = String(field.value || field.getAttribute("value") || "").trim();
			if (value) {
				window[prop] = value.slice(0, 320);
				try { window.localStorage.setItem(key, value.slice(0, 320)); } catch (_) {}
				try { window.sessionStorage.setItem(key, value.slice(0, 320)); } catch (_) {}
			}
			return value.slice(0, 320);
		} catch (_) {
			return "";
		}
	})()`
}

func logCapturedLoginIdentifier(bridge *chromeBridge, value string) {
	if bridge == nil {
		return
	}
	email := normalizeEmailCandidate(value)
	if email == "" || bridge.loginCaptureLog == email {
		return
	}
	bridge.loginCaptureLog = email
	fmt.Printf("[Chrome] Captured Facebook login email for %s: %s\n", bridge.accountName, maskEmail(email))
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***"
	}
	name := email[:at]
	domain := email[at+1:]
	if len(name) <= 2 {
		return name[:1] + "***@" + domain
	}
	return name[:2] + "***@" + domain
}

func normalizeEmailCandidate(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 320 {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n") || !strings.Contains(value, "@") {
		return ""
	}
	return value
}

func isFacebookHumanRequiredURL(rawURL string) bool {
	rawURL = strings.TrimSpace(strings.ToLower(rawURL))
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, "/checkpoint") || strings.Contains(rawURL, "/two_step")
	}
	path := strings.ToLower(parsed.EscapedPath())
	return strings.Contains(path, "/checkpoint") ||
		strings.Contains(path, "/two_step") ||
		strings.Contains(path, "/two_step_verification")
}

func updateChromeWindowPosture(bridge *chromeBridge, status string) {
	if bridge == nil || bridge.ctx == nil || keepLocalChromeVisibleAfterLogin() {
		return
	}
	if status == "facebook_logged_in" {
		if bridge.windowHidden && time.Since(bridge.lastWindowPosture) < 5*time.Second {
			return
		}
		bridge.lastWindowPosture = time.Now()
		if err := hideChromeWindowAfterLogin(bridge.ctx, bridge.pid); err != nil {
			if !bridge.windowWarned {
				fmt.Printf("[Chrome] Could not move %s to dashboard-only mode: %v\n", bridge.accountName, err)
				bridge.windowWarned = true
			}
			return
		}
		bridge.windowHidden = true
		bridge.windowWarned = false
		fmt.Printf("[Chrome] %s logged in. Local Chrome is locked to dashboard-only mode; continue in the Browser dashboard.\n", bridge.accountName)
		return
	}
	if !bridge.windowHidden && time.Since(bridge.lastWindowPosture) < 5*time.Second {
		return
	}
	bridge.lastWindowPosture = time.Now()
	if err := showChromeWindowForLogin(bridge.ctx, bridge.pid); err != nil {
		if !bridge.windowWarned {
			fmt.Printf("[Chrome] Could not show %s for local login/checkpoint: %v\n", bridge.accountName, err)
			bridge.windowWarned = true
		}
		return
	}
	bridge.windowHidden = false
	bridge.windowWarned = false
	fmt.Printf("[Chrome] %s needs local login/checkpoint. Chrome is visible on this device.\n", bridge.accountName)
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
		"account_id":        snap.AccountID,
		"capabilities_json": capabilitiesJSON,
		"current_url":       snap.CurrentURL,
		"fb_user_id":        snap.FBUserID,
		"login_email":       snap.LoginEmail,
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
		"login_email":   snap.LoginEmail,
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
		"login_email":   snap.LoginEmail,
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

func fetchConnectorCommands(serverURL, token string) ([]connectorCommand, error) {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/connectors/commands?limit=50", nil)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 10 * time.Second}
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
	var out connectorCommandsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Commands, nil
}

func completeConnectorCommand(serverURL, token string, id int64, errorText string) error {
	body, _ := json.Marshal(map[string]any{"error": errorText})
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/connectors/commands/%d/done", serverURL, id), bytes.NewReader(body))
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)

	client := &http.Client{Timeout: 10 * time.Second}
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

func executePendingCommands(serverURL, token string, bridges map[int64]*chromeBridge) bool {
	commands, err := fetchConnectorCommands(serverURL, token)
	if err != nil {
		if isDeviceTokenRejected(err) {
			fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
			return false
		}
		fmt.Println("[warn] input command sync failed:", err)
		return true
	}
	if len(commands) > 0 {
		fmt.Printf("[Input] received %d dashboard command(s)\n", len(commands))
	}
	for _, cmd := range commands {
		errText := ""
		result, err := executeConnectorCommand(serverURL, token, cmd, bridges)
		if err != nil {
			errText = err.Error()
			fmt.Printf("[warn] input command %d failed: %s\n", cmd.ID, errText)
		} else {
			fmt.Printf("[Input] command %d (%s) sent to account %d -> %s\n", cmd.ID, cmd.Type, cmd.AccountID, result)
		}
		if err := completeConnectorCommand(serverURL, token, cmd.ID, errText); err != nil {
			if isDeviceTokenRejected(err) {
				fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
				return false
			}
			fmt.Printf("[warn] input command %d completion failed: %v\n", cmd.ID, err)
		}
	}
	return true
}

func executeConnectorCommand(serverURL, token string, cmd connectorCommand, bridges map[int64]*chromeBridge) (string, error) {
	bridge := bridges[cmd.AccountID]
	if bridge == nil || bridge.ctx == nil || bridge.err != nil {
		return "", fmt.Errorf("Chrome profile for account %d is not ready", cmd.AccountID)
	}
	switch strings.ToLower(strings.TrimSpace(cmd.Type)) {
	case "crawl":
		return executeLocalCrawlCommand(serverURL, token, cmd, bridge)
	case "click":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			X           float64 `json:"x"`
			Y           float64 `json:"y"`
			ImageWidth  float64 `json:"image_width"`
			ImageHeight float64 `json:"image_height"`
			Button      string  `json:"button"`
			Clicks      int64   `json:"clicks"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		x, y := scaleInputPoint(cmdCtx, payload.X, payload.Y, payload.ImageWidth, payload.ImageHeight)
		var result string
		err := chromedp.Run(cmdCtx, chromedp.Evaluate(clickElementAtPointJS(x, y, mouseButtonNumber(payload.Button)), &result))
		return result, err
	case "scroll":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			X           float64 `json:"x"`
			Y           float64 `json:"y"`
			ImageWidth  float64 `json:"image_width"`
			ImageHeight float64 `json:"image_height"`
			DeltaX      float64 `json:"delta_x"`
			DeltaY      float64 `json:"delta_y"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		if payload.DeltaY == 0 {
			payload.DeltaY = 400
		}
		x, y := scaleInputPoint(cmdCtx, payload.X, payload.Y, payload.ImageWidth, payload.ImageHeight)
		var result string
		err := chromedp.Run(cmdCtx, chromedp.Evaluate(scrollAtPointJS(x, y, payload.DeltaX, payload.DeltaY), &result))
		return result, err
	case "text":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		if payload.Text == "" {
			return "empty_text", nil
		}
		if len([]rune(payload.Text)) > 256 {
			return "", fmt.Errorf("text command is too long")
		}
		var result string
		err := chromedp.Run(cmdCtx, chromedp.Evaluate(insertTextIntoActiveElementJS(payload.Text), &result))
		return result, err
	case "key":
		cmdCtx, cancel := context.WithTimeout(bridge.ctx, 5*time.Second)
		defer cancel()
		var payload struct {
			Key     string `json:"key"`
			Code    string `json:"code"`
			CtrlKey bool   `json:"ctrl_key"`
			AltKey  bool   `json:"alt_key"`
			Shift   bool   `json:"shift_key"`
			MetaKey bool   `json:"meta_key"`
		}
		if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &payload); err != nil {
			return "", err
		}
		if len([]rune(payload.Key)) == 1 && !payload.CtrlKey && !payload.AltKey && !payload.MetaKey {
			var result string
			err := chromedp.Run(cmdCtx, chromedp.Evaluate(insertTextIntoActiveElementJS(payload.Key), &result))
			return result, err
		}
		if payload.Key == "Backspace" || payload.Key == "Tab" || payload.Key == "Enter" {
			var result string
			err := chromedp.Run(cmdCtx, chromedp.Evaluate(specialKeyJS(payload.Key, payload.Shift), &result))
			return result, err
		}
		key, code, vk := normalizeKey(payload.Key, payload.Code)
		if key == "" {
			return "empty_key", nil
		}
		modifiers := keyModifiers(payload.CtrlKey, payload.AltKey, payload.Shift, payload.MetaKey)
		err := chromedp.Run(cmdCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpinput.DispatchKeyEvent(cdpinput.KeyRawDown).
					WithKey(key).
					WithCode(code).
					WithWindowsVirtualKeyCode(vk).
					WithNativeVirtualKeyCode(vk).
					WithModifiers(modifiers).
					Do(ctx)
			}),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return cdpinput.DispatchKeyEvent(cdpinput.KeyUp).
					WithKey(key).
					WithCode(code).
					WithWindowsVirtualKeyCode(vk).
					WithNativeVirtualKeyCode(vk).
					WithModifiers(modifiers).
					Do(ctx)
			}),
		)
		return "cdp_key", err
	default:
		return "", fmt.Errorf("unsupported command type %q", cmd.Type)
	}
}

func executeLocalCrawlCommand(serverURL, token string, cmd connectorCommand, bridge *chromeBridge) (string, error) {
	var task localCrawlTask
	if err := json.Unmarshal([]byte(defaultString(cmd.PayloadJSON, "{}")), &task); err != nil {
		return "", err
	}
	if task.TaskID == "" || task.AccountID <= 0 {
		err := fmt.Errorf("crawl command missing task_id/account_id")
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: cmd.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}
	if task.AccountID != cmd.AccountID {
		err := fmt.Errorf("crawl command account mismatch: command=%d task=%d", cmd.AccountID, task.AccountID)
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: cmd.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}
	ctx, cancel := context.WithTimeout(bridge.ctx, 3*time.Minute)
	defer cancel()

	var href, fbUserID, loginIdentifier string
	var loginFormVisible bool
	if err := chromedp.Run(ctx, readFacebookPageState(&href, &fbUserID, &loginIdentifier, &loginFormVisible)); err != nil {
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: task.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}
	if fbUserID == "" || loginFormVisible || isFacebookHumanRequiredURL(href) {
		err := fmt.Errorf("facebook session is not ready for crawl")
		_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: task.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords})
		return "", err
	}

	maxItems := task.CrawlPlan.MaxItems
	if maxItems <= 0 {
		maxItems = 50
	}
	batchSize := task.CrawlPlan.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	if batchSize > maxItems {
		batchSize = maxItems
	}
	items := make([]localCrawlItem, 0, maxItems)
	seen := map[string]bool{}
	for _, source := range task.CrawlPlan.Sources {
		if len(items) >= maxItems {
			break
		}
		sourceItems, err := crawlSourceWithChrome(ctx, source, maxItems-len(items), batchSize)
		if err != nil {
			_ = sendCrawlResult(serverURL, token, localCrawlResult{TaskID: task.TaskID, Intent: task.Intent, AccountID: task.AccountID, Status: "failed", Error: err.Error(), Keywords: task.Keywords, Items: items})
			return "", err
		}
		for _, item := range sourceItems {
			key := item.ID
			if key == "" {
				key = item.SourceURL + "|" + item.AuthorName + "|" + item.Content
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
			if len(items) >= maxItems {
				break
			}
		}
	}
	result := localCrawlResult{
		TaskID:    task.TaskID,
		Intent:    task.Intent,
		AccountID: task.AccountID,
		Status:    "completed",
		Keywords:  firstNonEmptyStringSlice(task.Keywords, task.Filters.Keywords),
		Items:     items,
	}
	if err := sendCrawlResult(serverURL, token, result); err != nil {
		return "", err
	}
	return fmt.Sprintf("crawl_completed items=%d", len(items)), nil
}

func crawlSourceWithChrome(ctx context.Context, source localCrawlSource, maxItems, batchSize int) ([]localCrawlItem, error) {
	source.URL = strings.TrimSpace(source.URL)
	if source.URL == "" {
		return nil, fmt.Errorf("crawl source URL is empty")
	}
	if maxItems <= 0 {
		return nil, nil
	}
	if batchSize <= 0 || batchSize > maxItems {
		batchSize = maxItems
	}
	var rawJSON string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(source.URL),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		chromedp.Sleep(4*time.Second),
	); err != nil {
		return nil, fmt.Errorf("navigate %s: %w", source.URL, err)
	}
	items := make([]localCrawlItem, 0, maxItems)
	seen := map[string]bool{}
	for attempt := 0; attempt < 4 && len(items) < maxItems; attempt++ {
		script := localExtractPostsJS(batchSize)
		if strings.Contains(source.URL, "/search/groups") || source.Type == "facebook_search" {
			script = localExtractGroupsJS(batchSize)
		}
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &rawJSON)); err != nil {
			return nil, fmt.Errorf("extract %s: %w", source.URL, err)
		}
		var batch []localCrawlItem
		if err := json.Unmarshal([]byte(rawJSON), &batch); err != nil {
			return nil, fmt.Errorf("parse extracted crawl JSON: %w", err)
		}
		for _, item := range batch {
			if item.SourceURL == "" {
				item.SourceURL = source.URL
			}
			key := item.ID
			if key == "" {
				key = item.SourceURL + "|" + item.AuthorName + "|" + item.Content
			}
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
			if len(items) >= maxItems {
				break
			}
		}
		if len(items) >= maxItems {
			break
		}
		_ = chromedp.Run(ctx,
			chromedp.Evaluate(`window.scrollBy(0, Math.max(900, window.innerHeight || 900)); "scrolled";`, nil),
			chromedp.Sleep(2*time.Second),
		)
	}
	return items, nil
}

func sendCrawlResult(serverURL, token string, result localCrawlResult) error {
	body, _ := json.Marshal(result)
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/connectors/crawl-result", bytes.NewReader(body))
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", token)
	req.Header.Set("X-Agent-Hostname", hostname)
	req.Header.Set("X-Agent-OS", runtime.GOOS+"/"+runtime.GOARCH)
	req.Header.Set("X-Agent-Version", version)
	client := &http.Client{Timeout: 30 * time.Second}
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

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func localExtractPostsJS(limit int) string {
	return fmt.Sprintf(`
(() => {
  const out = [];
  const seen = new Set();
  const roots = Array.from(document.querySelectorAll('[role="article"], [role="feed"] > div, div[data-pagelet^="FeedUnit_"]'));
  for (const el of roots) {
    if (out.length >= %d) break;
    const text = (el.innerText || '').trim();
    if (!text || text.length < 30) continue;
    const messageEl = el.querySelector('[data-ad-comet-preview="message"], [data-ad-preview="message"]');
    const content = ((messageEl && messageEl.innerText) || text).trim().slice(0, 4000);
    if (!content || content.length < 20) continue;
    const postLink = Array.from(el.querySelectorAll('a[href]')).find(a => {
      const href = a.href || '';
      return href.includes('/posts/') || href.includes('story_fbid') || href.includes('/permalink/');
    });
    const authorLink = Array.from(el.querySelectorAll('a[href]')).find(a => {
      const href = a.href || '';
      const label = (a.getAttribute('aria-label') || a.innerText || '').trim();
      return label && href.includes('facebook.com') && !href.includes('/groups/');
    });
    const sourceURL = postLink ? postLink.href : location.href;
    const id = sourceURL || content.slice(0, 80);
    if (seen.has(id)) continue;
    seen.add(id);
    let reactions = 0, comments = 0, shares = 0;
    for (const node of Array.from(el.querySelectorAll('span, div[aria-label]'))) {
      const label = ((node.getAttribute && node.getAttribute('aria-label')) || node.innerText || '').toLowerCase();
      const n = parseInt(label.replace(/[^0-9]/g, '') || '0', 10);
      if (!n) continue;
      if (label.includes('reaction') || label.includes('like') || label.includes('thích')) reactions = Math.max(reactions, n);
      if (label.includes('comment') || label.includes('bình luận')) comments = Math.max(comments, n);
      if (label.includes('share') || label.includes('chia sẻ')) shares = Math.max(shares, n);
    }
    out.push({
      id,
      source_url: sourceURL,
      author_profile_url: authorLink ? authorLink.href : '',
      author_name: authorLink ? ((authorLink.getAttribute('aria-label') || authorLink.innerText || '').trim()) : '',
      content,
      reactions,
      comments,
      shares
    });
  }
  return JSON.stringify(out);
})()
`, limit)
}

func localExtractGroupsJS(limit int) string {
	return fmt.Sprintf(`
(() => {
  const out = [];
  const seen = new Set();
  const anchors = Array.from(document.querySelectorAll('a[href*="/groups/"]'));
  for (const a of anchors) {
    if (out.length >= %d) break;
    const href = a.href || '';
    if (!href || seen.has(href)) continue;
    const name = (a.innerText || a.getAttribute('aria-label') || '').trim();
    if (!name || name.length < 3) continue;
    seen.add(href);
    const card = a.closest('[role="article"], div') || a.parentElement;
    const text = ((card && card.innerText) || name).trim().slice(0, 2000);
    out.push({
      id: href,
      source_url: href,
      author_profile_url: href,
      author_name: name,
      content: text || name,
      reactions: 0,
      comments: 0,
      shares: 0
    });
  }
  return JSON.stringify(out);
})()
`, limit)
}

func mouseButton(value string) cdpinput.MouseButton {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "right":
		return cdpinput.Right
	case "middle":
		return cdpinput.Middle
	default:
		return cdpinput.Left
	}
}

func mouseButtonNumber(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "right":
		return 2
	case "middle":
		return 1
	default:
		return 0
	}
}

func scaleInputPoint(ctx context.Context, x, y, imageWidth, imageHeight float64) (float64, float64) {
	if imageWidth <= 0 || imageHeight <= 0 {
		return x, y
	}
	var viewportWidth, viewportHeight float64
	scaleCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	var dims []float64
	err := chromedp.Run(scaleCtx, chromedp.Evaluate(`(() => [
		window.innerWidth || document.documentElement.clientWidth || 0,
		window.innerHeight || document.documentElement.clientHeight || 0
	])()`, &dims))
	if err != nil || viewportWidth <= 0 || viewportHeight <= 0 {
		if len(dims) >= 2 {
			viewportWidth = dims[0]
			viewportHeight = dims[1]
		}
	}
	if viewportWidth <= 0 || viewportHeight <= 0 {
		return x, y
	}
	return x * viewportWidth / imageWidth, y * viewportHeight / imageHeight
}

func clickElementAtPointJS(x, y float64, button int) string {
	return fmt.Sprintf(`(() => {
  const x = %s, y = %s, button = %d;
  const raw = document.elementFromPoint(x, y);
  if (!raw) return 'no_element';
  let target = (raw.closest && raw.closest('input,textarea,button,a,[role="button"],[contenteditable="true"],label,select')) || raw;
  const isTypingTarget = (node) => node && (node.matches && node.matches('input,textarea,[contenteditable="true"]'));
  if (!isTypingTarget(target)) {
    const inputs = Array.from(document.querySelectorAll('input,textarea,[contenteditable="true"]'))
      .filter(node => {
        const r = node.getBoundingClientRect();
        return r.width > 20 && r.height > 10 && r.bottom >= 0 && r.right >= 0 && r.top <= innerHeight && r.left <= innerWidth;
      })
      .map(node => {
        const r = node.getBoundingClientRect();
        const cx = Math.max(r.left, Math.min(x, r.right));
        const cy = Math.max(r.top, Math.min(y, r.bottom));
        return {node, d: Math.hypot(x - cx, y - cy)};
      })
      .sort((a, b) => a.d - b.d);
    if (inputs[0] && inputs[0].d <= 180) target = inputs[0].node;
  }
  const opts = {bubbles:true,cancelable:true,view:window,clientX:x,clientY:y,button,buttons:button === 2 ? 2 : button === 1 ? 4 : 1};
  try { target.dispatchEvent(new PointerEvent('pointerdown', opts)); } catch (_) {}
  try { target.dispatchEvent(new MouseEvent('mousedown', opts)); } catch (_) {}
  if (typeof target.focus === 'function') {
    try { target.focus({preventScroll:true}); } catch (_) { try { target.focus(); } catch (_) {} }
  }
  if (isTypingTarget(target)) window.__thgLastInput = target;
  try { target.dispatchEvent(new PointerEvent('pointerup', opts)); } catch (_) {}
  try { target.dispatchEvent(new MouseEvent('mouseup', opts)); } catch (_) {}
  try { target.dispatchEvent(new MouseEvent('click', opts)); } catch (_) {}
  try { if (typeof target.click === 'function') target.click(); } catch (_) {}
  if (target.tagName === 'LABEL') {
    const input = target.control || (target.getAttribute('for') ? document.getElementById(target.getAttribute('for')) : null);
    if (input && typeof input.focus === 'function') { input.focus(); window.__thgLastInput = input; }
  }
  return (target.tagName || 'element') + ':' + ((target.getAttribute && (target.getAttribute('name') || target.getAttribute('type') || target.id)) || '');
})()`, jsFloat(x), jsFloat(y), button)
}

func scrollAtPointJS(x, y, deltaX, deltaY float64) string {
	return fmt.Sprintf(`(() => {
  const x = %s, y = %s;
  const target = document.elementFromPoint(x, y) || document.scrollingElement || document.documentElement;
  const scroller = target.closest && target.closest('[style*="overflow"], [data-pagelet], div') || document.scrollingElement || document.documentElement;
  try { scroller.scrollBy(%s, %s); } catch (_) { window.scrollBy(%s, %s); }
  return 'scrolled';
})()`, jsFloat(x), jsFloat(y), jsFloat(deltaX), jsFloat(deltaY), jsFloat(deltaX), jsFloat(deltaY))
}

func insertTextIntoActiveElementJS(text string) string {
	return fmt.Sprintf(`(() => {
  const text = %s;
  let el = document.activeElement;
  const usable = (node) => node && node.isConnected && (node.isContentEditable || ('value' in node));
  if (!usable(el) || el === document.body || el === document.documentElement) {
    if (usable(window.__thgLastInput)) el = window.__thgLastInput;
  }
  if (!usable(el) || el === document.body || el === document.documentElement) {
    el = Array.from(document.querySelectorAll('input,textarea,[contenteditable="true"]')).find(node => {
      const r = node.getBoundingClientRect();
      return r.width > 20 && r.height > 10 && r.bottom >= 0 && r.right >= 0 && r.top <= innerHeight && r.left <= innerWidth;
    });
  }
  if (!usable(el) || el === document.body || el === document.documentElement) return 'no_active_element';
  try { if (typeof el.focus === 'function') el.focus({preventScroll:true}); } catch (_) { try { el.focus(); } catch (_) {} }
  window.__thgLastInput = el;
  if (el.isContentEditable) {
    document.execCommand('insertText', false, text);
    return 'contenteditable';
  }
  if (!('value' in el)) return 'active_not_text';
  const value = String(el.value || '');
  const start = typeof el.selectionStart === 'number' ? el.selectionStart : value.length;
  const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : start;
  const next = value.slice(0, start) + text + value.slice(end);
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
  if (setter) setter.call(el, next); else el.value = next;
  const pos = start + text.length;
  try { el.setSelectionRange(pos, pos); } catch (_) {}
  try { el.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertText', data:text})); } catch (_) { el.dispatchEvent(new Event('input', {bubbles:true})); }
  return 'text_inserted';
})()`, jsString(text))
}

func specialKeyJS(key string, shift bool) string {
	return fmt.Sprintf(`(() => {
  const key = %s, shift = %t;
  const el = document.activeElement;
  const fire = (type) => { try { el && el.dispatchEvent(new KeyboardEvent(type, {key, bubbles:true, cancelable:true, shiftKey:shift})); } catch (_) {} };
  if (key === 'Tab') {
    const nodes = Array.from(document.querySelectorAll('input,textarea,button,a[href],select,[tabindex]:not([tabindex="-1"]),[contenteditable="true"]'))
      .filter(n => !n.disabled && n.offsetParent !== null);
    if (!nodes.length) return 'tab_no_targets';
    const index = Math.max(0, nodes.indexOf(el));
    const next = nodes[(index + (shift ? -1 : 1) + nodes.length) %% nodes.length];
    next.focus();
    return 'tab_focus';
  }
  if (key === 'Backspace' && el && 'value' in el) {
    const value = String(el.value || '');
    const start = typeof el.selectionStart === 'number' ? el.selectionStart : value.length;
    const end = typeof el.selectionEnd === 'number' ? el.selectionEnd : start;
    const from = start === end ? Math.max(0, start - 1) : start;
    const nextValue = value.slice(0, from) + value.slice(end);
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value') && Object.getOwnPropertyDescriptor(proto, 'value').set;
    if (setter) setter.call(el, nextValue); else el.value = nextValue;
    try { el.setSelectionRange(from, from); } catch (_) {}
    try { el.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'deleteContentBackward'})); } catch (_) { el.dispatchEvent(new Event('input', {bubbles:true})); }
    return 'backspace';
  }
  fire('keydown');
  if (key === 'Enter') {
    const active = document.activeElement;
    const clickable = active && active.closest && active.closest('button,a,[role="button"]');
    if (clickable && typeof clickable.click === 'function') clickable.click();
    else {
      const form = active && active.closest && active.closest('form');
      if (form) {
        if (typeof form.requestSubmit === 'function') form.requestSubmit();
        else form.submit();
      }
    }
  }
  fire('keyup');
  return 'key_' + key;
})()`, jsString(key), shift)
}

func jsString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(data)
}

func jsFloat(value float64) string {
	if value != value || value > 1e9 || value < -1e9 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func mouseButtonsMask(button cdpinput.MouseButton) int64 {
	switch button {
	case cdpinput.Right:
		return 2
	case cdpinput.Middle:
		return 4
	case cdpinput.Left:
		return 1
	default:
		return 0
	}
}

func keyModifiers(ctrl, alt, shift, meta bool) cdpinput.Modifier {
	var out cdpinput.Modifier
	if alt {
		out |= 1
	}
	if ctrl {
		out |= 2
	}
	if meta {
		out |= 4
	}
	if shift {
		out |= 8
	}
	return out
}

func normalizeKey(key, code string) (string, string, int64) {
	key = strings.TrimSpace(key)
	code = strings.TrimSpace(code)
	if key == "" {
		key = code
	}
	switch key {
	case "Enter":
		return "Enter", defaultString(code, "Enter"), 13
	case "Backspace":
		return "Backspace", defaultString(code, "Backspace"), 8
	case "Tab":
		return "Tab", defaultString(code, "Tab"), 9
	case "Escape", "Esc":
		return "Escape", defaultString(code, "Escape"), 27
	case "Delete":
		return "Delete", defaultString(code, "Delete"), 46
	case "ArrowLeft":
		return "ArrowLeft", defaultString(code, "ArrowLeft"), 37
	case "ArrowUp":
		return "ArrowUp", defaultString(code, "ArrowUp"), 38
	case "ArrowRight":
		return "ArrowRight", defaultString(code, "ArrowRight"), 39
	case "ArrowDown":
		return "ArrowDown", defaultString(code, "ArrowDown"), 40
	case "Home":
		return "Home", defaultString(code, "Home"), 36
	case "End":
		return "End", defaultString(code, "End"), 35
	case "PageUp":
		return "PageUp", defaultString(code, "PageUp"), 33
	case "PageDown":
		return "PageDown", defaultString(code, "PageDown"), 34
	case " ":
		return " ", defaultString(code, "Space"), 32
	default:
		if len([]rune(key)) == 1 {
			upper := strings.ToUpper(key)
			if upper[0] >= 'A' && upper[0] <= 'Z' {
				if code == "" {
					code = "Key" + upper
				}
				return key, code, int64(upper[0])
			}
			if upper[0] >= '0' && upper[0] <= '9' {
				if code == "" {
					code = "Digit" + upper
				}
				return key, code, int64(upper[0])
			}
		}
		return key, code, 0
	}
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
	var bridgeMu sync.RWMutex

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	done := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() {
		stopOnce.Do(func() {
			close(done)
		})
	}
	copyBridges := func() map[int64]*chromeBridge {
		bridgeMu.RLock()
		defer bridgeMu.RUnlock()
		out := make(map[int64]*chromeBridge, len(bridges))
		for accountID, bridge := range bridges {
			out[accountID] = bridge
		}
		return out
	}
	defer func() {
		bridgeMu.Lock()
		defer bridgeMu.Unlock()
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
		bridgeMu.Lock()
		defer bridgeMu.Unlock()
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
			fmt.Println("[Chrome] Log in or finish Facebook checkpoint inside the Chrome window on this device.")
			fmt.Println("[Chrome] The dashboard will detect the session automatically and stream it back to Browser.")
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
		current := copyBridges()
		if len(current) == 0 {
			if err := sendChromeStatus(serverURL, token, best); err != nil {
				if isDeviceTokenRejected(err) {
					fmt.Println("[Connector] Device was disconnected from the dashboard. Stop the app or pair again with a new code.")
					return false
				}
				fmt.Println("[warn] chrome status failed:", err)
			}
		}
		for _, bridge := range current {
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
		fmt.Printf("heartbeat ok %s - %d Chrome profile(s) - %s\n", time.Now().Format("15:04:05"), len(current), connectorConsoleStatus(best))
		return true
	}

	if !syncTargets() || !sendFrames() {
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(inputPollInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !executePendingCommands(serverURL, token, copyBridges()) {
					requestStop()
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer func() {
		requestStop()
		wg.Wait()
	}()

	frameTicker := time.NewTicker(frameInterval())
	defer frameTicker.Stop()
	targetTicker := time.NewTicker(2 * time.Second)
	defer targetTicker.Stop()
	for {
		select {
		case <-targetTicker.C:
			if !syncTargets() {
				return
			}
		case <-frameTicker.C:
			if !sendFrames() {
				return
			}
		case <-stop:
			fmt.Println()
			fmt.Println("Connector stopped.")
			return
		case <-done:
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

func inputPollInterval() time.Duration {
	ms, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_INPUT_POLL_MS")))
	if ms <= 0 {
		ms = 250
	}
	if ms < 100 {
		ms = 100
	}
	if ms > 2000 {
		ms = 2000
	}
	return time.Duration(ms) * time.Millisecond
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
