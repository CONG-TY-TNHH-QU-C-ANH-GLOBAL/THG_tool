package server

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/workspace"
)

// workspaceList returns all Facebook accounts with their live Docker container status.
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
		VNCPort     int        `json:"vnc_port,omitempty"`
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
				e.VNCPort = inst.VNCPort
				t := inst.StartedAt
				e.StartedAt = &t
			}
		}
		result = append(result, e)
	}
	return c.JSON(fiber.Map{"workspaces": result, "count": len(result)})
}

// workspaceStart launches a Docker container for a specific account and waits
// until the VNC port inside the container is connectable (up to 45s).
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

	inst, err := s.workspace.Start(id, acc.Name)
	if err != nil {
		log.Printf("[Workspace] Failed to start container for account %d: %v", id, err)
		return c.Status(500).JSON(fiber.Map{"error": fmt.Sprintf("container start failed: %v", err)})
	}

	// Block until x11vnc inside the container is listening on the host-mapped port.
	// Container startup (Xvfb + x11vnc + Chrome) typically takes 5-15s.
	if !workspace.WaitForVNC(inst.VNCPort, 45*time.Second) {
		s.workspace.Stop(id)
		return c.Status(500).JSON(fiber.Map{
			"error": "container started but VNC did not become ready — check the Docker image: docker build -t thg-browser ./docker/",
		})
	}

	_ = s.db.UpdateAccountStatus(id, models.AccountActive)

	log.Printf("[Workspace] Account %d (%s) browser ready, vnc=127.0.0.1:%d", id, acc.Name, inst.VNCPort)
	return c.JSON(fiber.Map{
		"status":   "running",
		"vnc_port": inst.VNCPort,
	})
}

// workspaceStop kills the Docker container for a specific account.
// POST /api/browser/workspaces/:id/stop
func (s *Server) workspaceStop(c *fiber.Ctx) error {
	id, _ := strconv.ParseInt(c.Params("id"), 10, 64)
	if s.workspace != nil {
		s.workspace.Stop(id)
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}

// workspaceNavigate is a no-op in Docker/VNC mode.
// Navigation happens directly in the browser via mouse/keyboard through noVNC.
// POST /api/browser/workspaces/:id/navigate
func (s *Server) workspaceNavigate(c *fiber.Ctx) error {
	return c.Status(501).JSON(fiber.Map{
		"error": "navigate is not available in VNC mode — use the browser directly via the dashboard",
	})
}

