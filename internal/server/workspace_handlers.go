package server

import (
	"log"
	"runtime"
	"strconv"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
)

// workspaceList returns all Facebook accounts with their live Chrome workspace status.
// GET /api/browser/workspaces
func (s *Server) workspaceList(c *fiber.Ctx) error {
	orgID, _ := c.Locals("org_id").(int64)
	accounts, err := s.db.GetAllAccounts(orgID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	type entry struct {
		AccountID   int64      `json:"account_id"`
		AccountName string     `json:"account_name"`
		Status      string     `json:"account_status"`
		Running     bool       `json:"running"`
		CDPPort     int        `json:"cdp_port,omitempty"`
		StartedAt   *time.Time `json:"started_at,omitempty"`
	}

	result := make([]entry, 0, len(accounts))
	for _, acc := range accounts {
		e := entry{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Status:      string(acc.Status),
		}
		if s.workspace != nil {
			if inst := s.workspace.Get(acc.ID); inst != nil {
				e.Running = true
				e.CDPPort = inst.CDPPort
				t := inst.StartedAt
				e.StartedAt = &t
			}
		}
		result = append(result, e)
	}
	return c.JSON(fiber.Map{"workspaces": result, "count": len(result)})
}

// workspaceStart launches Chrome for a specific account and waits until CDP is ready.
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

	// On Linux production servers: ensure Xvfb virtual display is running before Chrome
	if runtime.GOOS == "linux" && s.vncDisplay != nil {
		if err := s.vncDisplay.Start(); err != nil {
			log.Printf("[Workspace] Xvfb start warning: %v", err)
		}
	}

	inst, err := s.workspace.Start(id, acc.Name)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	cdpPort := inst.CDPPort

	// Block until Chrome's CDP port responds (up to 15s) so the frontend knows it's truly ready
	wsURL, waitErr := waitForChromeWS(cdpPort)
	if waitErr != nil {
		s.workspace.Stop(id)
		return c.Status(500).JSON(fiber.Map{
			"error": "Chrome started but CDP did not become ready — check Chrome installation or profile path",
		})
	}
	_ = wsURL

	// Update account status to active now that Chrome is running
	_ = s.db.UpdateAccountStatus(id, models.AccountActive)

	// Start CDP screencast in background
	go s.startAccountScreencast(id, cdpPort)

	log.Printf("[Workspace] Account %d (%s) workspace ready, cdp=%d", id, acc.Name, cdpPort)
	return c.JSON(fiber.Map{
		"status":   "running",
		"cdp_port": cdpPort,
	})
}

// workspaceStop kills Chrome for a specific account.
// POST /api/browser/workspaces/:id/stop
func (s *Server) workspaceStop(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	s.stopAccountScreencast(id)
	if s.workspace != nil {
		s.workspace.Stop(id)
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}

// workspaceNavigate navigates the account's Chrome to a URL.
// POST /api/browser/workspaces/:id/navigate
func (s *Server) workspaceNavigate(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	var req struct {
		URL string `json:"url"`
	}
	if err := c.BodyParser(&req); err != nil || req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "url required"})
	}

	hub := s.getAccountHub(id)
	if hub == nil {
		return c.Status(503).JSON(fiber.Map{"error": "browser not connected — start it first"})
	}
	hub.mu.RLock()
	ctx := hub.cdpCtx
	hub.mu.RUnlock()
	if ctx == nil {
		return c.Status(503).JSON(fiber.Map{"error": "browser not connected — start it first"})
	}

	go chromedp.Run(ctx, chromedp.Navigate(req.URL))
	return c.JSON(fiber.Map{"status": "navigating", "url": req.URL})
}

