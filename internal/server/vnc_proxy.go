package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
)

// Package-level singleton for the legacy single-Chrome workspace (non-Docker mode).
var (
	workspaceMu   sync.Mutex
	workspaceProc *exec.Cmd
)

// ── Per-account VNC proxy (Docker mode) ──────────────────────────────────────

// perAccountVNCProxyHandler handles GET /ws/vnc/:id
// It looks up the VNC host port for the running container and proxies the
// WebSocket connection directly to the x11vnc TCP socket inside Docker.
func (s *Server) perAccountVNCProxyHandler() func(*fiberws.Conn) {
	return func(ws *fiberws.Conn) {
		accountID, err := strconv.ParseInt(ws.Params("id"), 10, 64)
		if err != nil {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("invalid account id"))
			return
		}

		if s.workspace == nil {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("workspace manager not initialized"))
			return
		}

		inst := s.workspace.Get(accountID)
		if inst == nil || inst.VNCPort == 0 {
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("browser not running — start it first"))
			return
		}

		vncAddr := fmt.Sprintf("127.0.0.1:%d", inst.VNCPort)
		log.Printf("[VNCProxy] Account %d → %s", accountID, vncAddr)
		proxyVNC(ws, vncAddr)
	}
}

// proxyVNC bridges a WebSocket connection to a raw TCP VNC server.
// It is the shared implementation used by both per-account and legacy handlers.
func proxyVNC(ws *fiberws.Conn, vncAddr string) {
	tcp, err := net.DialTimeout("tcp", vncAddr, 8*time.Second)
	if err != nil {
		log.Printf("[VNCProxy] Cannot reach VNC at %s: %v", vncAddr, err)
		_ = ws.WriteMessage(fiberws.TextMessage,
			[]byte("VNC server not reachable — container may still be starting, retry in a moment"))
		return
	}
	defer tcp.Close()

	log.Printf("[VNCProxy] Tunnel open: WebSocket ↔ %s", vncAddr)
	errc := make(chan error, 2)

	// VNC TCP → WebSocket (binary frames)
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if werr := ws.WriteMessage(fiberws.BinaryMessage, buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				errc <- err
				return
			}
		}
	}()

	// WebSocket → VNC TCP
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if _, werr := tcp.Write(data); werr != nil {
				errc <- werr
				return
			}
		}
	}()

	<-errc
	log.Printf("[VNCProxy] Tunnel closed: %s", vncAddr)
}

// ── Legacy single-display VNC proxy (kept for backward compatibility) ─────────

// vncStatus returns VNC + browser state for the dashboard.
// GET /api/browser/status
func (s *Server) vncStatus(c *fiber.Ctx) error {
	workspaceMu.Lock()
	running := workspaceProc != nil && workspaceProc.Process != nil
	workspaceMu.Unlock()

	vncRunning := s.vncDisplay != nil && s.vncDisplay.IsRunning()
	return c.JSON(fiber.Map{
		"browser_running": running,
		"vnc_running":     vncRunning,
		"vnc_port":        s.cfg.VNCPort,
		"cdp_port":        s.cfg.CDPPort,
		"display":         s.vncDisplay.Display(),
	})
}

// vncStart launches Xvfb + x11vnc + Chrome (legacy single-display mode).
// POST /api/browser/start
func (s *Server) vncStart(c *fiber.Ctx) error {
	if s.vncDisplay == nil {
		return c.Status(503).JSON(fiber.Map{"error": "VNC not configured"})
	}
	if err := s.vncDisplay.Start(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	go func() {
		time.Sleep(time.Second)
		s.startWorkspaceChrome()
		time.Sleep(2 * time.Second)
		go s.startAccountScreencast(0, s.cfg.CDPPort)
	}()
	return c.JSON(fiber.Map{"status": "starting", "vnc_port": s.cfg.VNCPort})
}

// vncStop kills Chrome + VNC display (legacy mode).
// POST /api/browser/stop
func (s *Server) vncStop(c *fiber.Ctx) error {
	s.stopWorkspaceChrome()
	if s.vncDisplay != nil {
		s.vncDisplay.Stop()
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}

// vncProxyHandler proxies WebSocket ↔ TCP for the legacy single VNC display.
// GET /ws/vnc  (kept for backward compatibility)
func (s *Server) vncProxyHandler() func(*fiberws.Conn) {
	vncAddr := fmt.Sprintf("127.0.0.1:%d", s.cfg.VNCPort)
	return func(ws *fiberws.Conn) {
		proxyVNC(ws, vncAddr)
	}
}

// startWorkspaceChrome launches Chrome in the legacy VNC virtual display.
func (s *Server) startWorkspaceChrome() {
	workspaceMu.Lock()
	defer workspaceMu.Unlock()

	if workspaceProc != nil && workspaceProc.Process != nil {
		return
	}
	chromePath := s.resolveChromePath()
	cdpPort := s.cfg.CDPPort
	if cdpPort == 0 {
		cdpPort = 9222
	}
	args := []string{
		"--no-first-run", "--no-default-browser-check",
		"--disable-notifications", "--disable-infobars",
		"--disable-blink-features=AutomationControlled",
		"--no-sandbox", "--disable-dev-shm-usage",
		fmt.Sprintf("--user-data-dir=%s/workspace", s.cfg.ProfileDir),
		fmt.Sprintf("--remote-debugging-port=%d", cdpPort),
		"--remote-debugging-address=127.0.0.1",
		"--window-size=1280,800",
		"about:blank",
	}
	cmd := exec.Command(chromePath, args...)
	if s.vncDisplay != nil {
		cmd.Env = append(cmd.Environ(), "DISPLAY="+s.vncDisplay.Display())
	}
	if err := cmd.Start(); err != nil {
		log.Printf("[Browser] Chrome start failed: %v", err)
		return
	}
	workspaceProc = cmd
	go func() {
		_ = cmd.Wait()
		workspaceMu.Lock()
		if workspaceProc == cmd {
			workspaceProc = nil
		}
		workspaceMu.Unlock()
	}()
}

func (s *Server) stopWorkspaceChrome() {
	s.stopAccountScreencast(0)
	workspaceMu.Lock()
	defer workspaceMu.Unlock()
	if workspaceProc != nil && workspaceProc.Process != nil {
		workspaceProc.Process.Kill()
		workspaceProc = nil
	}
}

// Discard prevents unused import error.
var _ = io.Discard
