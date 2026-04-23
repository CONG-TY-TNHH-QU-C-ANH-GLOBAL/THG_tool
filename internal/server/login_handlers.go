package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
)

type loginSession struct {
	port   int
	cancel context.CancelFunc
}

var loginSessions sync.Map // map[int64]*loginSession

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
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return "", fmt.Errorf("chrome not ready: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &info); err != nil || info.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("cannot parse chrome debug endpoint")
	}
	return info.WebSocketDebuggerURL, nil
}

// cdpContext connects to the running Chrome and returns a ready chromedp context.
// The returned cancel must always be called.
func cdpContext(port int, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	wsURL, err := chromeBrowserWS(port)
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

// startLoginSession launches a headless Chrome with the account's profile and remote debugging.
// POST /api/accounts/:id/start-login
func (s *Server) startLoginSession(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}

	// Kill any existing session for this account
	if old, ok := loginSessions.Load(id); ok {
		old.(*loginSession).cancel()
		loginSessions.Delete(id)
		time.Sleep(600 * time.Millisecond)
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

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("remote-debugging-port", fmt.Sprintf("%d", port)),
		chromedp.Flag("remote-debugging-address", "127.0.0.1"),
		chromedp.ExecPath(s.resolveChromePath()),
		chromedp.UserDataDir(profileDir),
		chromedp.WindowSize(1280, 800),
	)

	// Keep the session running for up to 10 minutes
	ctx, cancel := context.WithTimeout(allocCtx, 10*time.Minute)

	// Create context to start Chrome
	bCtx, bCancel := chromedp.NewContext(ctx)

	// Navigate to facebook
	if err := chromedp.Run(bCtx, chromedp.Navigate("https://www.facebook.com/login")); err != nil {
		bCancel()
		cancel()
		allocCancel()
		return c.Status(500).JSON(fiber.Map{"error": "failed to start Chrome: " + err.Error()})
	}

	// We pass a composite cancel function that stops the browser and allocator
	fullCancel := func() {
		bCancel()
		cancel()
		allocCancel()
	}

	sess := &loginSession{port: port, cancel: fullCancel}
	loginSessions.Store(id, sess)

	go func() {
		<-ctx.Done()
		loginSessions.Delete(id)
		fullCancel()
		log.Printf("[Login] Chrome session ended for account %d.", id)
	}()

	log.Printf("[Login] Chrome started for account %d on port %d (profile: %s)", id, port, profileDir)
	return c.JSON(fiber.Map{
		"port":         port,
		"status":       "starting",
		"account_name": account.Name,
		"tunnel":       fmt.Sprintf("ssh -L %d:127.0.0.1:%d -p 24700 -N root@103.216.117.194", port, port),
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
