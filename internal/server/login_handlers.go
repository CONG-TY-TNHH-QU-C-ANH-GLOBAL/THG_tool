package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
)

type loginSession struct {
	cmd    *exec.Cmd
	port   int
	cancel context.CancelFunc
}

var loginSessions sync.Map // map[int64]*loginSession

// maxConcurrentSessions prevents OOM on VPS when users click Login multiple times.
const maxConcurrentSessions = 5

type cdpEndpoint struct {
	BaseURL string
	WSHost  string
	Label   string
}

func cdpEndpointFromPort(port int) cdpEndpoint {
	host := fmt.Sprintf("127.0.0.1:%d", port)
	return cdpEndpoint{
		BaseURL: "http://" + host,
		WSHost:  host,
		Label:   host,
	}
}

func findFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// chromeBrowserWS returns the browser-level WebSocket URL from Chrome's debug endpoint.
func chromeBrowserWS(port int) (string, error) {
	return chromeBrowserWSFromEndpoint(cdpEndpointFromPort(port))
}

func chromeBrowserWSFromEndpoint(ep cdpEndpoint) (string, error) {
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

// cdpContext connects to the running Chrome and returns a ready chromedp context.
// The returned cancel must always be called.
func cdpContext(port int, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	return cdpContextForEndpoint(cdpEndpointFromPort(port), timeout)
}

func cdpContextForEndpoint(ep cdpEndpoint, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	wsURL, err := chromeBrowserWSFromEndpoint(ep)
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

func (s *Server) resolveChromePath() string {
	if s.cfg.ChromePath != "" {
		return s.cfg.ChromePath
	}

	switch runtime.GOOS {
	case "windows":
		for _, p := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Chromium\Application\chrome.exe`,
			`C:\Program Files (x86)\Chromium\Application\chrome.exe`,
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		// Fallback: try "chrome" which may be on PATH via Windows App Paths registry
		if p, err := exec.LookPath("chrome"); err == nil {
			return p
		}
		return "chrome"

	case "darwin":
		for _, p := range []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "google-chrome"

	default: // linux
		for _, p := range []string{
			"/usr/bin/chromium-browser",
			"/usr/bin/chromium",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/google-chrome",
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "chromium-browser"
	}
}

// startLoginSession launches a headless Chrome with the account's profile and remote debugging.
// POST /api/accounts/:id/start-login
func (s *Server) startLoginSession(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Enforce global cap to prevent OOM from multiple concurrent Chrome processes
	if old, ok := loginSessions.Load(id); ok {
		old.(*loginSession).cancel()
		loginSessions.Delete(id)
		time.Sleep(600 * time.Millisecond)
	} else {
		count := 0
		loginSessions.Range(func(_, _ any) bool { count++; return true })
		if count >= maxConcurrentSessions {
			return c.Status(429).JSON(fiber.Map{"error": "too many Chrome sessions active — please stop another session first"})
		}
	}

	account, err := s.db.GetAccount(id)
	if err != nil || account == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}

	port, err := findFreePort()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "no free port available"})
	}

	profileDir := fmt.Sprintf("%s/account_%d", s.cfg.ProfileDir, id)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "cannot create profile dir"})
	}

	chromePath := s.resolveChromePath()
	log.Printf("[Login] Chrome path resolved: %q (OS: %s, headless: %v)", chromePath, runtime.GOOS, s.cfg.Headless)

	// Build Chrome launch args — add --headless=new when running on a VPS without display
	chromArgs := []string{
		"--user-data-dir=" + profileDir,
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--disable-gpu",
		"--disable-blink-features=AutomationControlled",
		"--no-first-run",
		"--disable-default-apps",
		"--window-size=1280,800",
		"about:blank",
	}
	if s.cfg.Headless {
		chromArgs = append([]string{"--headless=new"}, chromArgs...)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	cmd := exec.CommandContext(ctx, chromePath, chromArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		log.Printf("[Login] ❌ Chrome start failed (path=%q): %v", chromePath, err)
		return c.Status(500).JSON(fiber.Map{
			"error":       fmt.Sprintf("Không thể mở Chrome: %v", err),
			"chrome_path": chromePath,
			"hint":        "Kiểm tra Chrome đã được cài đặt, hoặc set CHROME_PATH trong .env",
		})
	}
	log.Printf("[Login] ✅ Chrome PID=%d started for account %d on port %d", cmd.Process.Pid, id, port)

	// Navigate to Facebook login page after Chrome is ready
	go func() {
		time.Sleep(2 * time.Second)
		bCtx, bCancel, err := cdpContext(port, 30*time.Second)
		if err == nil {
			defer bCancel()
			_ = chromedp.Run(bCtx, chromedp.Navigate("https://www.facebook.com/login"))
		}
	}()

	sess := &loginSession{cmd: cmd, port: port, cancel: cancel}
	loginSessions.Store(id, sess)

	go func() {
		err := cmd.Wait()
		loginSessions.Delete(id)
		cancel()
		log.Printf("[Login] Chrome session ended for account %d. Process error: %v", id, err)
	}()

	// Build SSH tunnel command using configurable server host
	serverHost := s.cfg.ServerHost
	if serverHost == "" {
		serverHost = c.Hostname()
	}
	sshPort := s.cfg.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	tunnelCmd := fmt.Sprintf("ssh -L %d:127.0.0.1:%d root@%s -p %d -N", port, port, serverHost, sshPort)

	log.Printf("[Login] Chrome started for account %d on port %d (headless=%v, profile: %s)", id, port, s.cfg.Headless, profileDir)
	return c.JSON(fiber.Map{
		"port":          port,
		"status":        "starting",
		"account_name":  account.Name,
		"headless":      s.cfg.Headless,
		"tunnel":        tunnelCmd,
		"devtools_host": fmt.Sprintf("localhost:%d", port),
	})
}

// loginStatus polls the Chrome session for a Facebook c_user cookie.
// GET /api/accounts/:id/login-status
func (s *Server) loginStatus(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	sessI, ok := loginSessions.Load(id)
	if !ok {
		return c.JSON(fiber.Map{"status": "no_session", "logged_in": false})
	}
	sess := sessI.(*loginSession)

	ctx, cancel, err := cdpContext(sess.port, 6*time.Second)
	if err != nil {
		return c.JSON(fiber.Map{"status": "starting", "logged_in": false})
	}
	defer cancel()

	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		cookies, e = network.GetCookies().WithURLs([]string{
			"https://www.facebook.com",
			"https://facebook.com",
		}).Do(ctx)
		return e
	})); err != nil {
		return c.JSON(fiber.Map{"status": "checking", "logged_in": false})
	}

	for _, ck := range cookies {
		if ck.Name == "c_user" && ck.Value != "" {
			return c.JSON(fiber.Map{
				"status":     "logged_in",
				"logged_in":  true,
				"fb_user_id": ck.Value,
			})
		}
	}
	return c.JSON(fiber.Map{"status": "waiting", "logged_in": false})
}

// captureLoginSession reads all Facebook cookies from Chrome, saves them encrypted, stops Chrome.
// POST /api/accounts/:id/capture-session
func (s *Server) captureLoginSession(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	sessI, ok := loginSessions.Load(id)
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "no active session — start Chrome login first"})
	}
	sess := sessI.(*loginSession)

	ctx, cancel, err := cdpContext(sess.port, 10*time.Second)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "Chrome not ready: " + err.Error()})
	}
	defer cancel()

	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		cookies, e = network.GetCookies().WithURLs([]string{
			"https://www.facebook.com",
			"https://facebook.com",
		}).Do(ctx)
		return e
	})); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to read cookies: " + err.Error()})
	}

	// Check for c_user before saving — don't save unauthenticated cookies
	var fbUserID string
	for _, ck := range cookies {
		if ck.Name == "c_user" {
			fbUserID = ck.Value
			break
		}
	}
	if fbUserID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "not logged in yet — no c_user cookie found"})
	}

	type exportCookie struct {
		Name     string  `json:"name"`
		Value    string  `json:"value"`
		Domain   string  `json:"domain"`
		Path     string  `json:"path"`
		Expires  float64 `json:"expires,omitempty"`
		HTTPOnly bool    `json:"httpOnly"`
		Secure   bool    `json:"secure"`
	}
	out := make([]exportCookie, 0, len(cookies))
	for _, ck := range cookies {
		out = append(out, exportCookie{
			Name: ck.Name, Value: ck.Value, Domain: ck.Domain,
			Path: ck.Path, Expires: float64(ck.Expires),
			HTTPOnly: bool(ck.HTTPOnly), Secure: bool(ck.Secure),
		})
	}
	cookiesJSON, err := json.Marshal(out)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "serialization failed"})
	}

	if err := s.db.UpdateAccountCookies(id, string(cookiesJSON)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "save failed: " + err.Error()})
	}
	_ = s.db.UpdateAccountStatus(id, models.AccountActive)

	// Kill session
	sess.cancel()
	loginSessions.Delete(id)

	adminID, _ := c.Locals("user_id").(int64)
	_ = s.db.InsertAuditLog(adminID, "session_captured", c.IP(),
		fmt.Sprintf(`{"account_id":%d,"cookies":%d,"fb_user":"%s"}`, id, len(cookies), fbUserID))
	log.Printf("[Login] Captured %d cookies for account %d (fb_user=%s)", len(cookies), id, fbUserID)

	return c.JSON(fiber.Map{
		"status":        "saved",
		"cookies_count": len(cookies),
		"fb_user_id":    fbUserID,
	})
}

// stopLoginSession kills the Chrome session without saving cookies.
// POST /api/accounts/:id/stop-login
func (s *Server) stopLoginSession(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if sessI, ok := loginSessions.Load(id); ok {
		sessI.(*loginSession).cancel()
		loginSessions.Delete(id)
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}
