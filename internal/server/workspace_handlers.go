package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/workspace"
)

// workspaceList returns all Facebook accounts with their live browser status.
// GET /api/browser/workspaces
func (s *Server) workspaceList(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	accounts, err := s.db.GetAllAccounts(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	type entry struct {
		AccountID    int64      `json:"account_id"`
		AccountName  string     `json:"account_name"`
		Status       string     `json:"account_status"`
		LoggedIn     bool       `json:"logged_in"`
		FBUserID     string     `json:"fb_user_id,omitempty"`
		Running      bool       `json:"running"`
		CDPPort      int        `json:"cdp_port,omitempty"`
		VNCPort      int        `json:"vnc_port,omitempty"`
		StartedAt    *time.Time `json:"started_at,omitempty"`
		BrowserState string     `json:"browser_state,omitempty"`
		ErrorMsg     string     `json:"error_msg,omitempty"`
	}

	appStore, _ := store.NewAppStore(s.db)
	result := make([]entry, 0, len(accounts))
	for _, acc := range accounts {
		e := entry{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Status:      string(acc.Status),
			LoggedIn:    acc.BrowserLoggedIn,
			FBUserID:    acc.FBUserID,
		}
		if s.workspace != nil {
			if inst := s.workspace.Get(acc.ID); inst != nil {
				e.Running = true
				e.CDPPort = inst.CDPPort
				e.VNCPort = inst.VNCPort
				t := inst.StartedAt
				e.StartedAt = &t
			}
		}
		if appStore != nil {
			if sess, err := appStore.GetSession(c.Context(), acc.ID); err == nil && sess != nil && sess.Status != "terminated" {
				e.BrowserState = sess.Status
				e.ErrorMsg = sess.ErrorMsg
				if e.CDPPort == 0 {
					e.CDPPort = sess.CDPPort
				}
				if e.VNCPort == 0 {
					e.VNCPort = sess.VNCPort
				}
				if e.StartedAt == nil {
					t := sess.StartedAt
					e.StartedAt = &t
				}
			}
		}
		result = append(result, e)
	}
	return c.JSON(fiber.Map{"workspaces": result, "count": len(result)})
}

// workspaceStart launches a Docker browser for a specific account.
// Readiness is tracked asynchronously so production proxies never time out.
// POST /api/browser/workspaces/:id/start
func (s *Server) workspaceStart(c *fiber.Ctx) error {
	if s.workspace == nil {
		return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
	}
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	acc, err := s.db.GetAccount(id)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	orgID, _ := c.Locals("org_id").(int64)
	if orgID != 0 && acc.OrgID != orgID {
		return c.Status(403).JSON(fiber.Map{"error": "access denied"})
	}

	inst, err := s.workspace.Start(id, acc.Name)
	if err != nil {
		log.Printf("[Workspace] Failed to start container for account %d: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("container start failed: %v", err)})
	}

	_ = s.db.UpdateAccountStatus(id, models.AccountActive)
	s.recordBrowserSession(id, orgID, inst, "initializing", "")
	go s.watchWorkspaceReadiness(id, orgID, inst)
	go s.watchWorkspaceLogin(id, orgID, inst)

	log.Printf("[Workspace] Account %d (%s) browser starting, vnc=%d cdp=%d", id, acc.Name, inst.VNCPort, inst.CDPPort)
	return c.JSON(fiber.Map{
		"status":     "starting",
		"account_id": id,
		"vnc_port":   inst.VNCPort,
		"cdp_port":   inst.CDPPort,
	})
}

// workspaceStop kills the Docker browser for a specific account.
// POST /api/browser/workspaces/:id/stop
func (s *Server) workspaceStop(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	if acc, err := s.db.GetAccount(id); err == nil && acc != nil {
		orgID, _ := c.Locals("org_id").(int64)
		if orgID != 0 && acc.OrgID != orgID {
			return c.Status(403).JSON(fiber.Map{"error": "access denied"})
		}
	}
	if s.workspace != nil {
		s.workspace.Stop(id)
	}
	if appStore, err := store.NewAppStore(s.db); err == nil {
		_ = appStore.TerminateSession(context.Background(), id)
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}

// workspaceNew creates a fresh Facebook account and starts its browser.
// It returns as soon as Docker launches; CDP/VNC readiness is tracked async.
// POST /api/browser/workspaces/new
func (s *Server) workspaceNew(c *fiber.Ctx) error {
	if s.workspace == nil {
		return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
	}
	orgID, _ := c.Locals("org_id").(int64)
	userID, _ := c.Locals("user_id").(int64)

	name := fmt.Sprintf("Facebook %s", time.Now().Format("02/01 15:04"))
	acc := &models.Account{
		OrgID:          orgID,
		Platform:       models.PlatformFacebook,
		Name:           name,
		Status:         models.AccountInactive,
		AssignedUserID: userID,
	}
	id, err := s.db.AddAccount(acc)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "create account: " + err.Error()})
	}

	inst, err := s.workspace.Start(id, name)
	if err != nil {
		_ = s.db.DeleteAccount(id)
		log.Printf("[Workspace] Failed to start new session for account %d: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": "container start failed: " + err.Error()})
	}

	_ = s.db.UpdateAccountStatus(id, models.AccountActive)
	s.recordBrowserSession(id, orgID, inst, "initializing", "")
	go s.watchWorkspaceReadiness(id, orgID, inst)
	go s.watchWorkspaceLogin(id, orgID, inst)

	log.Printf("[Workspace] New session starting: account %d (%s) vnc=%d cdp=%d", id, name, inst.VNCPort, inst.CDPPort)
	return c.JSON(fiber.Map{
		"status":     "starting",
		"account_id": id,
		"vnc_port":   inst.VNCPort,
		"cdp_port":   inst.CDPPort,
	})
}

func (s *Server) recordBrowserSession(accountID, orgID int64, inst *workspace.Instance, status, errorMsg string) {
	if inst == nil {
		return
	}
	appStore, err := store.NewAppStore(s.db)
	if err != nil {
		log.Printf("[Workspace] session store unavailable for account %d: %v", accountID, err)
		return
	}
	_ = appStore.UpsertSession(context.Background(), store.BrowserSession{
		AccountID:    accountID,
		OrgID:        orgID,
		Status:       status,
		CDPPort:      inst.CDPPort,
		VNCPort:      inst.VNCPort,
		StartedAt:    inst.StartedAt.UTC(),
		LastActiveAt: time.Now().UTC(),
		ErrorMsg:     errorMsg,
	})
}

func (s *Server) persistFacebookBrowserSession(accountID, orgID int64, inst *workspace.Instance, fbUserID, cookiesJSON string) error {
	if fbUserID == "" {
		return fmt.Errorf("facebook user id is empty")
	}
	if err := s.db.SetBrowserLoggedIn(accountID, true, fbUserID); err != nil {
		return err
	}
	if cookiesJSON != "" {
		if err := s.db.UpdateAccountCookies(accountID, cookiesJSON); err != nil {
			return fmt.Errorf("save cookies failed: %w", err)
		}
	}
	_ = s.db.UpdateAccountStatus(accountID, models.AccountActive)
	if appStore, err := store.NewAppStore(s.db); err == nil {
		sess, err := appStore.GetSession(context.Background(), accountID)
		if err == nil && sess != nil && sess.Status != "terminated" {
			sess.Status = "idle"
			sess.LastActiveAt = time.Now().UTC()
			sess.ErrorMsg = ""
			if inst != nil {
				sess.CDPPort = inst.CDPPort
				sess.VNCPort = inst.VNCPort
			}
			_ = appStore.UpsertSession(context.Background(), *sess)
		} else if inst != nil {
			s.recordBrowserSession(accountID, orgID, inst, "idle", "")
		}
	}
	return nil
}

func (s *Server) watchWorkspaceLogin(accountID, orgID int64, inst *workspace.Instance) {
	if inst == nil {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(15 * time.Minute)
	defer deadline.Stop()

	for {
		select {
		case <-deadline.C:
			return
		case <-ticker.C:
			if s.workspace == nil {
				return
			}
			current := s.workspace.Get(accountID)
			if current == nil || current.ContainerID != inst.ContainerID {
				return
			}
			acc, err := s.db.GetAccount(accountID)
			if err != nil || acc == nil {
				return
			}
			if acc.BrowserLoggedIn && acc.FBUserID != "" {
				return
			}
			fbUserID, cookiesJSON, _, err := facebookCookiesFromInstance(inst)
			if err != nil {
				continue
			}
			if acc.FBUserID != "" && acc.FBUserID != fbUserID {
				msg := "Facebook profile mismatch; create a separate account slot for this Facebook user"
				s.recordBrowserSession(accountID, orgID, inst, "error", msg)
				log.Printf("[Workspace] Account %d login mismatch stored=%s current=%s", accountID, acc.FBUserID, fbUserID)
				return
			}
			if err := s.persistFacebookBrowserSession(accountID, orgID, inst, fbUserID, cookiesJSON); err != nil {
				log.Printf("[Workspace] Account %d auto session persist failed: %v", accountID, err)
				continue
			}
			log.Printf("[Workspace] Account %d Facebook session auto-saved (fb_user_id=%s)", accountID, fbUserID)
			return
		}
	}
}

func (s *Server) watchWorkspaceReadiness(accountID, orgID int64, inst *workspace.Instance) {
	if inst == nil {
		return
	}

	vncCh := make(chan bool, 1)
	cdpCh := make(chan bool, 1)
	go func() { vncCh <- workspace.WaitForVNC(inst.VNCPort, 60*time.Second) }()
	go func() { cdpCh <- workspace.WaitForCDP(inst.CDPPort, 90*time.Second) }()

	vncReady := <-vncCh
	if vncReady {
		// VNC is the operator-facing live desktop. Mark it separately so the
		// dashboard can render the browser even while CDP is still warming up.
		s.recordBrowserSession(accountID, orgID, inst, "display_ready", "")
		log.Printf("[Workspace] Account %d browser desktop ready, vnc=%d cdp=%d", accountID, inst.VNCPort, inst.CDPPort)
	} else {
		cdpReady := <-cdpCh
		msg := "VNC did not become ready; check x11vnc/Xvfb in docker logs"
		if !cdpReady {
			msg = "VNC and Chrome CDP did not become ready; rebuild thg-browser and check docker logs"
		}
		s.recordBrowserSession(accountID, orgID, inst, "error", msg)
		log.Printf("[Workspace] Account %d browser startup warning: %s", accountID, msg)
		return
	}

	cdpReady := <-cdpCh
	if cdpReady {
		s.recordBrowserSession(accountID, orgID, inst, "ready", "")
		log.Printf("[Workspace] Account %d browser ready, vnc=%d cdp=%d", accountID, inst.VNCPort, inst.CDPPort)
		return
	}

	msg := "Desktop is visible, but Chrome CDP did not become ready; automation/login verification may wait. Check Chromium startup in docker logs"
	s.recordBrowserSession(accountID, orgID, inst, "display_ready", msg)
	log.Printf("[Workspace] Account %d browser startup warning: %s", accountID, msg)
}

// workspaceNavigate is a no-op in Docker/VNC mode.
// Navigation happens directly in the browser via mouse/keyboard through the viewer.
// POST /api/browser/workspaces/:id/navigate
func (s *Server) workspaceNavigate(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{
		"error": "navigate is not available in browser-view mode; use the browser directly via the dashboard",
	})
}

type facebookSessionSnapshot struct {
	AccountID    int64  `json:"account_id"`
	AccountName  string `json:"account_name"`
	LoggedIn     bool   `json:"logged_in"`
	FBUserID     string `json:"fb_user_id,omitempty"`
	StoredFBID   string `json:"stored_fb_user_id,omitempty"`
	CurrentURL   string `json:"current_url,omitempty"`
	CurrentTitle string `json:"current_title,omitempty"`
	Checkpoint   bool   `json:"checkpoint"`
	CookieError  string `json:"cookie_error,omitempty"`
	CookiesCount int    `json:"cookies_count,omitempty"`
	cookiesJSON  string
}

// workspaceSyncSession probes the running browser and persists the Facebook
// identity when the c_user cookie is available.
// POST /api/browser/workspaces/:id/sync-session
func (s *Server) workspaceSyncSession(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	acc, err := s.db.GetAccount(id)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	orgID, _ := c.Locals("org_id").(int64)
	if orgID != 0 && acc.OrgID != orgID {
		return c.Status(403).JSON(fiber.Map{"error": "access denied"})
	}
	if s.workspace == nil {
		return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
	}
	inst := s.workspace.Get(id)
	if inst == nil || inst.CDPPort == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "browser is not running"})
	}

	snap, err := facebookSessionSnapshotFromInstance(inst)
	if err != nil {
		return c.Status(503).JSON(fiber.Map{"error": "session probe failed: " + err.Error()})
	}
	snap.AccountID = id
	snap.AccountName = acc.Name
	snap.StoredFBID = acc.FBUserID

	if snap.FBUserID != "" {
		if acc.FBUserID != "" && acc.FBUserID != snap.FBUserID {
			return c.Status(409).JSON(fiber.Map{
				"error": "this browser profile is logged into a different Facebook user; use a separate account slot for multi-account automation",
			})
		}
		if err := s.persistFacebookBrowserSession(id, orgID, inst, snap.FBUserID, snap.cookiesJSON); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		snap.LoggedIn = true
		snap.StoredFBID = snap.FBUserID
	}

	return c.JSON(snap)
}

func facebookSessionSnapshotFromInstance(inst *workspace.Instance) (*facebookSessionSnapshot, error) {
	if inst == nil {
		return nil, fmt.Errorf("browser instance is not running")
	}
	var lastSnap *facebookSessionSnapshot
	var errors []string
	for _, ep := range cdpEndpointsForInstance(inst) {
		snap, err := facebookSessionSnapshotFromCDPEndpoint(ep)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", ep.Label, err))
			continue
		}
		if snap.CookieError == "" || snap.FBUserID != "" {
			return snap, nil
		}
		if !isCDPUnavailableMessage(snap.CookieError) {
			return snap, nil
		}
		lastSnap = snap
		errors = append(errors, fmt.Sprintf("%s: %s", ep.Label, snap.CookieError))
	}
	if lastSnap != nil {
		if len(errors) > 0 {
			lastSnap.CookieError = strings.Join(errors, "; ")
		}
		return lastSnap, nil
	}
	return nil, fmt.Errorf("no CDP endpoint succeeded: %s", strings.Join(errors, "; "))
}

func facebookSessionSnapshotFromCDP(cdpPort int) (*facebookSessionSnapshot, error) {
	return facebookSessionSnapshotFromCDPEndpoint(cdpEndpointFromPort(cdpPort))
}

func facebookSessionSnapshotFromCDPEndpoint(ep cdpEndpoint) (*facebookSessionSnapshot, error) {
	snap := &facebookSessionSnapshot{}
	targets, err := fetchCDPTargetsFromEndpoint(ep)
	if err != nil {
		snap.CookieError = "CDP target list unavailable: " + err.Error()
	} else {
		for _, t := range targets {
			if t.Type != "page" {
				continue
			}
			if strings.Contains(strings.ToLower(t.URL), "facebook.com") || snap.CurrentURL == "" {
				snap.CurrentURL = t.URL
				snap.CurrentTitle = t.Title
			}
		}
	}

	lower := strings.ToLower(snap.CurrentURL + " " + snap.CurrentTitle)
	snap.Checkpoint = strings.Contains(lower, "checkpoint") || strings.Contains(lower, "security")

	fbUserID, cookiesJSON, cookieCount, err := facebookCookiesFromCDPEndpoint(ep)
	if err != nil {
		if snap.CookieError != "" {
			snap.CookieError += "; cookies: " + err.Error()
		} else {
			snap.CookieError = err.Error()
		}
		return snap, nil
	}
	snap.FBUserID = fbUserID
	snap.LoggedIn = fbUserID != ""
	snap.CookiesCount = cookieCount
	snap.cookiesJSON = cookiesJSON
	return snap, nil
}

func cdpEndpointsForInstance(inst *workspace.Instance) []cdpEndpoint {
	if inst == nil {
		return nil
	}
	endpoints := []cdpEndpoint{}
	if inst.CDPPort > 0 {
		endpoints = append(endpoints, cdpEndpointFromPort(inst.CDPPort))
	}
	if ip := dockerContainerIP(inst); ip != "" {
		host := net.JoinHostPort(ip, "9222")
		endpoints = append(endpoints, cdpEndpoint{
			BaseURL: "http://" + host,
			WSHost:  host,
			Label:   "container " + host,
		})
	}
	return endpoints
}

func dockerContainerIP(inst *workspace.Instance) string {
	if inst == nil || inst.ContainerID == "" {
		return ""
	}
	out, err := exec.Command("docker", "inspect", "--format={{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", inst.ContainerID).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// workspaceSetLoggedIn marks whether an account has successfully logged into Facebook.
// POST /api/browser/workspaces/:id/set-logged-in
func (s *Server) workspaceSetLoggedIn(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	acc, err := s.db.GetAccount(id)
	if err != nil || acc == nil {
		return c.Status(404).JSON(fiber.Map{"error": "account not found"})
	}
	orgID, _ := c.Locals("org_id").(int64)
	if orgID != 0 && acc.OrgID != orgID {
		return c.Status(403).JSON(fiber.Map{"error": "access denied"})
	}
	var body struct {
		LoggedIn bool `json:"logged_in"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}

	var fbUserID string
	var cookiesJSON string
	if body.LoggedIn {
		if s.workspace == nil {
			return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
		}
		inst := s.workspace.Get(id)
		if inst == nil || inst.CDPPort == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "browser is not running"})
		}
		var cookieCount int
		var err error
		fbUserID, cookiesJSON, cookieCount, err = facebookCookiesFromInstance(inst)
		if err != nil {
			if isCDPUnavailable(err) {
				msg := "CDP endpoint is not reachable from the API host; restart this browser session after the thg-browser image is rebuilt"
				s.recordBrowserSession(id, acc.OrgID, inst, "display_ready", msg+": "+err.Error())
				return c.Status(503).JSON(fiber.Map{
					"error": "browser CDP is not ready: " + err.Error(),
					"code":  "CDP_UNAVAILABLE",
					"hint":  msg,
				})
			}
			return c.Status(400).JSON(fiber.Map{"error": "not logged in to Facebook yet: " + err.Error()})
		}
		_ = cookieCount
		if acc.FBUserID != "" && acc.FBUserID != fbUserID {
			return c.Status(409).JSON(fiber.Map{
				"error": "this account profile is logged into a different Facebook user; create another Facebook account slot for multi-account automation, or clear this profile before reusing it",
			})
		}
	}

	if body.LoggedIn {
		if err := s.persistFacebookBrowserSession(id, orgID, s.workspace.Get(id), fbUserID, cookiesJSON); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	} else if err := s.db.SetBrowserLoggedIn(id, false); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "logged_in": body.LoggedIn, "fb_user_id": fbUserID})
}

func facebookUserIDFromCDP(cdpPort int) (string, error) {
	fbUserID, _, _, err := facebookCookiesFromCDP(cdpPort)
	return fbUserID, err
}

func facebookCookiesFromCDP(cdpPort int) (string, string, int, error) {
	return facebookCookiesFromCDPEndpoint(cdpEndpointFromPort(cdpPort))
}

func facebookCookiesFromInstance(inst *workspace.Instance) (string, string, int, error) {
	if inst == nil {
		return "", "", 0, fmt.Errorf("browser instance is not running")
	}
	var errors []string
	for _, ep := range cdpEndpointsForInstance(inst) {
		fbUserID, cookiesJSON, cookieCount, err := facebookCookiesFromCDPEndpoint(ep)
		if err == nil {
			return fbUserID, cookiesJSON, cookieCount, nil
		}
		if isMissingFacebookUserCookie(err) {
			return "", "", cookieCount, err
		}
		errors = append(errors, fmt.Sprintf("%s: %v", ep.Label, err))
	}
	return "", "", 0, fmt.Errorf("no CDP endpoint succeeded: %s", strings.Join(errors, "; "))
}

func facebookCookiesFromCDPEndpoint(ep cdpEndpoint) (string, string, int, error) {
	ctx, cancel, err := cdpContextForEndpoint(ep, 8*time.Second)
	if err != nil {
		return "", "", 0, err
	}
	defer cancel()

	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, network.Enable(), chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		cookies, e = network.GetCookies().WithURLs([]string{
			"https://www.facebook.com",
			"https://facebook.com",
		}).Do(ctx)
		return e
	})); err != nil {
		return "", "", 0, err
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
	var fbUserID string
	for _, ck := range cookies {
		if ck.Name == "c_user" && ck.Value != "" {
			fbUserID = ck.Value
		}
		out = append(out, exportCookie{
			Name:     ck.Name,
			Value:    ck.Value,
			Domain:   ck.Domain,
			Path:     ck.Path,
			Expires:  float64(ck.Expires),
			HTTPOnly: bool(ck.HTTPOnly),
			Secure:   bool(ck.Secure),
		})
	}
	if fbUserID == "" {
		return "", "", len(cookies), fmt.Errorf("missing c_user cookie")
	}
	cookiesJSON, err := json.Marshal(out)
	if err != nil {
		return "", "", len(cookies), fmt.Errorf("serialize cookies: %w", err)
	}
	return fbUserID, string(cookiesJSON), len(cookies), nil
}

func isCDPUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return isCDPUnavailableMessage(err.Error())
}

func isCDPUnavailableMessage(message string) bool {
	msg := strings.ToLower(message)
	return strings.Contains(msg, "chrome not ready") ||
		strings.Contains(msg, "no cdp endpoint succeeded") ||
		strings.Contains(msg, "cdp target list unavailable") ||
		strings.Contains(msg, "/json/version") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof")
}

func isMissingFacebookUserCookie(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "missing c_user")
}

// resolveCheckpoint marks a session as ready after an operator manually passed the
// Facebook verification gate via the browser viewer.
// POST /api/browser/workspaces/:id/resolve-checkpoint
func (s *Server) resolveCheckpoint(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	sm := session.NewStateMachine(s.db.DB())
	cm := session.NewCheckpointManager(s.db.DB(), sm, nil)
	if err := cm.ResolveCheckpoint(c.Context(), id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "account_id": id, "status": "ready"})
}

// listCheckpoints returns all sessions currently awaiting human intervention.
// GET /api/browser/checkpoints
func (s *Server) listCheckpoints(c *fiber.Ctx) error {
	sm := session.NewStateMachine(s.db.DB())
	cm := session.NewCheckpointManager(s.db.DB(), sm, nil)
	pending, err := cm.PendingCheckpoints(c.Context())
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"checkpoints": pending, "count": len(pending)})
}
