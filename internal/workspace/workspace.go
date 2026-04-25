package workspace

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Instance represents one running Chrome process for a Facebook account.
type Instance struct {
	AccountID   int64
	AccountName string
	ProfileDir  string
	CDPPort     int
	Process     *exec.Cmd
	StartedAt   time.Time
}

// IsRunning reports whether the Chrome process is still alive.
func (i *Instance) IsRunning() bool {
	return i != nil && i.Process != nil && i.Process.Process != nil
}

// Manager owns all per-account Chrome instances.
// Safe for concurrent use.
type Manager struct {
	mu          sync.RWMutex
	instances   map[int64]*Instance
	profileBase string
	chromePath  string
}

// NewManager creates a workspace manager.
// chromePath: path to the chrome/chromium binary
// profileBase: root dir for per-account Chrome profiles (e.g. "data/profiles")
func NewManager(chromePath, profileBase string) *Manager {
	if profileBase == "" {
		profileBase = filepath.Join(".", "data", "profiles")
	}
	_ = os.MkdirAll(profileBase, 0755)
	return &Manager{
		instances:   make(map[int64]*Instance),
		profileBase: profileBase,
		chromePath:  chromePath,
	}
}

// ProfileDir returns the Chrome user-data-dir for an account.
func (m *Manager) ProfileDir(accountID int64) string {
	if accountID == 0 {
		return m.profileBase
	}
	return filepath.Join(m.profileBase, fmt.Sprintf("account_%d", accountID))
}

// Get returns the running instance for accountID, or nil if not running.
func (m *Manager) Get(accountID int64) *Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst := m.instances[accountID]
	if inst != nil && !inst.IsRunning() {
		return nil
	}
	return inst
}

// List returns all currently running instances.
func (m *Manager) List() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Instance, 0, len(m.instances))
	for _, v := range m.instances {
		if v.IsRunning() {
			out = append(out, v)
		}
	}
	return out
}

// Start launches Chrome for accountID if not already running.
// Returns the running instance (existing or newly started).
func (m *Manager) Start(accountID int64, accountName string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if inst, ok := m.instances[accountID]; ok && inst.IsRunning() {
		log.Printf("[Workspace] Chrome for account %d already running (cdp=%d)", accountID, inst.CDPPort)
		return inst, nil
	}

	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("find free port: %w", err)
	}

	profileDir := m.profileDirLocked(accountID)
	_ = os.MkdirAll(profileDir, 0755)

	// Remove Chrome singleton lock files left by crashed sessions — without this,
	// Chrome refuses to start a second time against the same profile directory.
	for _, lockFile := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
		_ = os.Remove(filepath.Join(profileDir, lockFile))
	}

	chromePath := m.chromePath
	if chromePath == "" {
		chromePath = defaultChromePath()
	}
	log.Printf("[Workspace] Chrome binary: %s", chromePath)

	args := []string{
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-notifications",
		"--disable-infobars",
		"--disable-blink-features=AutomationControlled",
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--window-size=1280,800",
		"--start-maximized",
	}

	// Linux/CI-specific flags — skip on Windows where GPU is needed for rendering
	if runtime.GOOS != "windows" {
		args = append(args,
			"--no-sandbox",
			"--disable-dev-shm-usage",
			"--disable-gpu",
		)
	}

	cmd := exec.Command(chromePath, args...)
	// On Linux: inject DISPLAY so Chrome connects to the virtual framebuffer (Xvfb).
	// Without this, Chrome exits silently on a headless VPS.
	if runtime.GOOS != "windows" {
		display := os.Getenv("DISPLAY")
		if display == "" {
			display = ":99" // default Xvfb display
		}
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("chrome start: %w", err)
	}

	inst := &Instance{
		AccountID:   accountID,
		AccountName: accountName,
		ProfileDir:  profileDir,
		CDPPort:     port,
		Process:     cmd,
		StartedAt:   time.Now(),
	}
	m.instances[accountID] = inst

	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		if cur, ok := m.instances[accountID]; ok && cur == inst {
			delete(m.instances, accountID)
		}
		m.mu.Unlock()
		log.Printf("[Workspace] Chrome exited for account %d (%s)", accountID, accountName)
	}()

	log.Printf("[Workspace] Chrome started for account %d (%s) — cdp=%d profile=%s",
		accountID, accountName, port, profileDir)
	return inst, nil
}

// Stop kills Chrome for accountID.
func (m *Manager) Stop(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inst, ok := m.instances[accountID]
	if !ok {
		return
	}
	if inst.IsRunning() {
		inst.Process.Process.Kill()
	}
	delete(m.instances, accountID)
	log.Printf("[Workspace] Chrome stopped for account %d", accountID)
}

// StopAll kills all running Chrome instances (called on server shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, inst := range m.instances {
		if inst.IsRunning() {
			inst.Process.Process.Kill()
		}
		delete(m.instances, id)
	}
	log.Println("[Workspace] All Chrome instances stopped")
}

func (m *Manager) profileDirLocked(accountID int64) string {
	if accountID == 0 {
		return m.profileBase
	}
	return filepath.Join(m.profileBase, fmt.Sprintf("account_%d", accountID))
}

// freePort finds an available TCP port on localhost.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// defaultChromePath returns the default Chrome binary path for the current OS.
func defaultChromePath() string {
	switch runtime.GOOS {
	case "windows":
		paths := []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return "chrome"
	case "darwin":
		return "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	default: // linux
		for _, p := range []string{"google-chrome", "google-chrome-stable", "chromium-browser", "chromium"} {
			if path, err := exec.LookPath(p); err == nil {
				return path
			}
		}
		return "google-chrome"
	}
}
