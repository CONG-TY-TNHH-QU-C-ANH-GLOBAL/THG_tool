package server

import (
	"context"
	"fmt"
	"log"
	"strconv"
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
	if body.LoggedIn {
		if s.workspace == nil {
			return c.Status(503).JSON(fiber.Map{"error": "workspace manager not initialized"})
		}
		inst := s.workspace.Get(id)
		if inst == nil || inst.CDPPort == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "browser is not running"})
		}
		var err error
		fbUserID, err = facebookUserIDFromCDP(inst.CDPPort)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "not logged in to Facebook yet: " + err.Error()})
		}
		if acc.FBUserID != "" && acc.FBUserID != fbUserID {
			return c.Status(409).JSON(fiber.Map{
				"error": "this account profile is logged into a different Facebook user; create another Facebook account slot for multi-account automation, or clear this profile before reusing it",
			})
		}
	}

	if err := s.db.SetBrowserLoggedIn(id, body.LoggedIn, fbUserID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if body.LoggedIn {
		_ = s.db.UpdateAccountStatus(id, models.AccountActive)
		if appStore, err := store.NewAppStore(s.db); err == nil {
			if sess, err := appStore.GetSession(context.Background(), id); err == nil && sess != nil && sess.Status != "terminated" {
				sess.Status = "idle"
				sess.LastActiveAt = time.Now().UTC()
				if inst := s.workspace.Get(id); inst != nil {
					sess.CDPPort = inst.CDPPort
					sess.VNCPort = inst.VNCPort
				}
				_ = appStore.UpsertSession(context.Background(), *sess)
			}
		}
	}
	return c.JSON(fiber.Map{"ok": true, "logged_in": body.LoggedIn, "fb_user_id": fbUserID})
}

func facebookUserIDFromCDP(cdpPort int) (string, error) {
	ctx, cancel, err := cdpContext(cdpPort, 8*time.Second)
	if err != nil {
		return "", err
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
		return "", err
	}

	for _, ck := range cookies {
		if ck.Name == "c_user" && ck.Value != "" {
			return ck.Value, nil
		}
	}
	return "", fmt.Errorf("missing c_user cookie")
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
