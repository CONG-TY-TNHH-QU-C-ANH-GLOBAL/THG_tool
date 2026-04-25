package browser

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// VNCDisplay manages a virtual X11 display + VNC server on Linux.
// On non-Linux platforms it is a no-op (dev machines use headless Chrome).
type VNCDisplay struct {
	mu       sync.Mutex
	display  string // e.g. ":99"
	vncPort  int    // e.g. 5900
	xvfb     *exec.Cmd
	x11vnc   *exec.Cmd
	running  bool
}

// NewVNCDisplay creates a manager for display :99 and VNC port vncPort.
func NewVNCDisplay(displayNum, vncPort int) *VNCDisplay {
	return &VNCDisplay{
		display: ":" + strconv.Itoa(displayNum),
		vncPort: vncPort,
	}
}

// Start launches Xvfb and x11vnc. Safe to call multiple times.
func (v *VNCDisplay) Start() error {
	if runtime.GOOS != "linux" {
		log.Println("[VNC] Non-Linux platform — skipping Xvfb/x11vnc startup")
		return nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.running {
		return nil
	}

	// Kill any leftover processes from a prior crash
	exec.Command("pkill", "-f", "Xvfb "+v.display).Run()
	exec.Command("pkill", "-f", "x11vnc.*"+v.display).Run()
	time.Sleep(500 * time.Millisecond)

	// Start Xvfb (virtual framebuffer)
	v.xvfb = exec.Command("Xvfb", v.display,
		"-screen", "0", "1280x800x24",
		"-ac", "+extension", "RANDR",
	)
	v.xvfb.Stdout = os.Stdout
	v.xvfb.Stderr = os.Stderr
	if err := v.xvfb.Start(); err != nil {
		return fmt.Errorf("start Xvfb: %w", err)
	}
	log.Printf("[VNC] Xvfb started on display %s (pid=%d)", v.display, v.xvfb.Process.Pid)
	time.Sleep(800 * time.Millisecond) // wait for Xvfb to be ready

	// Start x11vnc — listen on localhost only for security
	v.x11vnc = exec.Command("x11vnc",
		"-display", v.display,
		"-nopw",           // no VNC password (secured by our JWT-gated WS proxy)
		"-listen", "127.0.0.1",
		"-rfbport", strconv.Itoa(v.vncPort),
		"-forever",        // don't exit after first client disconnects
		"-quiet",
		"-shared",         // allow multiple viewers
		"-xkb",
	)
	v.x11vnc.Stdout = os.Stdout
	v.x11vnc.Stderr = os.Stderr
	if err := v.x11vnc.Start(); err != nil {
		v.xvfb.Process.Kill()
		return fmt.Errorf("start x11vnc: %w", err)
	}
	log.Printf("[VNC] x11vnc started on 127.0.0.1:%d (pid=%d)", v.vncPort, v.x11vnc.Process.Pid)

	// Set DISPLAY so any child process (Chrome) uses this display
	os.Setenv("DISPLAY", v.display)

	v.running = true
	return nil
}

// Stop kills Xvfb and x11vnc.
func (v *VNCDisplay) Stop() {
	if runtime.GOOS != "linux" {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.x11vnc != nil && v.x11vnc.Process != nil {
		v.x11vnc.Process.Kill()
	}
	if v.xvfb != nil && v.xvfb.Process != nil {
		v.xvfb.Process.Kill()
	}
	v.running = false
	log.Println("[VNC] Display stopped")
}

// IsRunning reports whether the display is active.
func (v *VNCDisplay) IsRunning() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.running
}

// Display returns the DISPLAY env value (e.g. ":99").
func (v *VNCDisplay) Display() string {
	return v.display
}

// VNCPort returns the TCP port x11vnc listens on.
func (v *VNCDisplay) VNCPort() int {
	return v.vncPort
}
