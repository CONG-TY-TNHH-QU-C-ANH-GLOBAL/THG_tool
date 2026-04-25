package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
)

// Package-level singleton for the browser workspace Chrome process.
// Only one Chrome instance runs at a time per server process.
var (
	workspaceMu   sync.Mutex
	workspaceProc *exec.Cmd
)

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

// vncStart launches Xvfb + x11vnc + Chrome if not already running.
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
		// Start CDP screencast after Chrome is ready
		time.Sleep(2 * time.Second)
		go s.startAccountScreencast(0, s.cfg.CDPPort)
	}()
	return c.JSON(fiber.Map{"status": "starting", "vnc_port": s.cfg.VNCPort})
}

// vncStop kills Chrome + VNC display.
// POST /api/browser/stop
func (s *Server) vncStop(c *fiber.Ctx) error {
	s.stopWorkspaceChrome()
	if s.vncDisplay != nil {
		s.vncDisplay.Stop()
	}
	return c.JSON(fiber.Map{"status": "stopped"})
}

// vncProxyHandler proxies WebSocket ↔ raw TCP to the VNC server so noVNC can connect.
// GET /ws/vnc   (auth handled by the Use() middleware registered in api.go)
func (s *Server) vncProxyHandler() func(*fiberws.Conn) {
	vncAddr := fmt.Sprintf("127.0.0.1:%d", s.cfg.VNCPort)

	return func(ws *fiberws.Conn) {
		tcp, err := net.DialTimeout("tcp", vncAddr, 5*time.Second)
		if err != nil {
			log.Printf("[VNCProxy] Cannot reach VNC at %s: %v", vncAddr, err)
			_ = ws.WriteMessage(fiberws.TextMessage, []byte("VNC server not running — start browser first"))
			return
		}
		defer tcp.Close()

		log.Printf("[VNCProxy] Client connected → %s", vncAddr)
		errc := make(chan error, 2)

		// VNC TCP → WebSocket binary frames
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
		log.Println("[VNCProxy] Client disconnected")
	}
}

// startWorkspaceChrome launches Chrome in the VNC virtual display.
func (s *Server) startWorkspaceChrome() {
	workspaceMu.Lock()
	defer workspaceMu.Unlock()

	if workspaceProc != nil && workspaceProc.Process != nil {
		log.Println("[Browser] Chrome already running")
		return
	}

	chromePath := s.resolveChromePath()
	cdpPort := s.cfg.CDPPort
	if cdpPort == 0 {
		cdpPort = 9222
	}

	args := []string{
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-notifications",
		"--disable-infobars",
		"--disable-blink-features=AutomationControlled",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		fmt.Sprintf("--user-data-dir=%s/workspace", s.cfg.ProfileDir),
		fmt.Sprintf("--remote-debugging-port=%d", cdpPort),
		"--remote-debugging-address=127.0.0.1",
		"--window-size=1280,800",
		"about:blank",
	}

	cmd := exec.Command(chromePath, args...)
	cmd.Env = append(os.Environ(), "DISPLAY="+s.vncDisplay.Display())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[Browser] Chrome start failed: %v", err)
		return
	}
	workspaceProc = cmd
	log.Printf("[Browser] Chrome started in display %s (pid=%d, cdp=%d)", s.vncDisplay.Display(), cmd.Process.Pid, cdpPort)

	go func() {
		_ = cmd.Wait()
		workspaceMu.Lock()
		if workspaceProc == cmd {
			workspaceProc = nil
		}
		workspaceMu.Unlock()
		log.Println("[Browser] Chrome exited")
	}()
}

// stopWorkspaceChrome kills the workspace Chrome if running.
func (s *Server) stopWorkspaceChrome() {
	s.stopAccountScreencast(0)
	workspaceMu.Lock()
	defer workspaceMu.Unlock()
	if workspaceProc != nil && workspaceProc.Process != nil {
		workspaceProc.Process.Kill()
		workspaceProc = nil
		log.Println("[Browser] Chrome stopped by user")
	}
}

// Discard: prevent unused import lint error
var _ = io.Discard
