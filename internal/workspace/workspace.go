package workspace

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const containerPrefix = "thg-browser-"

// Instance represents one running Docker container for a Facebook account.
type Instance struct {
	AccountID   int64
	AccountName string
	ProfileDir  string
	ContainerID string // short Docker container ID
	CDPPort     int    // host-side port mapped from container's CDP :9222
	VNCPort     int    // host-side port mapped from container's VNC :5900
	StartedAt   time.Time
}

// IsRunning reports whether the instance is tracked as active.
func (i *Instance) IsRunning() bool {
	return i != nil && i.ContainerID != ""
}

// Manager owns all per-account Docker containers.
// Safe for concurrent use.
type Manager struct {
	mu           sync.RWMutex
	instances    map[int64]*Instance
	profileBase  string
	dockerImage  string
	portRegistry *PortRegistry // nil = use random Docker ports (legacy mode)
}

// NewManager creates a Docker-based workspace manager.
// chromePath is ignored — Chrome runs inside the container.
// Call ReconcileRunning() after this if you want to re-attach containers
// that survived a server restart.
func NewManager(chromePath, profileBase string) *Manager {
	if profileBase == "" {
		profileBase = filepath.Join(".", "data", "profiles")
	}
	_ = os.MkdirAll(profileBase, 0755)

	// BROWSER_IMAGE env overrides the default image name
	image := os.Getenv("BROWSER_IMAGE")
	if image == "" {
		image = "thg-browser:latest"
	}

	return &Manager{
		instances:   make(map[int64]*Instance),
		profileBase: profileBase,
		dockerImage: image,
	}
}

// SetPortRegistry attaches a PortRegistry so containers get deterministic ports.
// Must be called before the first Start() call.
func (m *Manager) SetPortRegistry(pr *PortRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.portRegistry = pr
}

// ProfileDir returns the Chrome user-data-dir for an account (host-side path).
func (m *Manager) ProfileDir(accountID int64) string {
	if accountID == 0 {
		return m.profileBase
	}
	return filepath.Join(m.profileBase, fmt.Sprintf("account_%d", accountID))
}

func (m *Manager) profileDirLocked(accountID int64) string {
	return m.ProfileDir(accountID)
}

// Get returns the tracked instance for accountID, or nil if not running.
func (m *Manager) Get(accountID int64) *Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.instances[accountID]
}

// List returns all currently tracked instances.
func (m *Manager) List() []*Instance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Instance, 0, len(m.instances))
	for _, v := range m.instances {
		out = append(out, v)
	}
	return out
}

// Start launches a Docker container for accountID.
// If a container for this account already exists and is running, returns it immediately.
func (m *Manager) Start(accountID int64, accountName string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return existing alive container immediately
	if inst, ok := m.instances[accountID]; ok && m.containerAlive(inst.ContainerID) {
		log.Printf("[Workspace] Container for account %d already running (%s)", accountID, inst.ContainerID)
		return inst, nil
	}

	profileDir := m.profileDirLocked(accountID)
	_ = os.MkdirAll(profileDir, 0755)

	absProfile, err := filepath.Abs(profileDir)
	if err != nil {
		return nil, fmt.Errorf("resolve profile path: %w", err)
	}

	containerName := fmt.Sprintf("%s%d", containerPrefix, accountID)

	// Remove any leftover container from a previous crash or stop
	exec.Command("docker", "rm", "-f", containerName).Run() //nolint:errcheck

	// Determine ports: use PortRegistry if wired, otherwise let Docker assign randomly.
	var cdpPort, vncPort int
	if m.portRegistry != nil {
		var err error
		cdpPort, vncPort, err = m.portRegistry.ClaimPair(accountID)
		if err != nil {
			return nil, fmt.Errorf("claim ports: %w", err)
		}
	}

	// Build docker run args
	var portArgs []string
	if cdpPort > 0 && vncPort > 0 {
		portArgs = []string{
			"-p", fmt.Sprintf("127.0.0.1:%d:5900", vncPort),
			"-p", fmt.Sprintf("127.0.0.1:%d:9222", cdpPort),
		}
	} else {
		portArgs = []string{
			"-p", "127.0.0.1::5900",
			"-p", "127.0.0.1::9222",
		}
	}

	args := append([]string{"run", "-d", "--name", containerName}, portArgs...)
	args = append(args,
		"--shm-size=1g",
		"--cpus=1.0",
		"--memory=2g",
		"--memory-swap=2g",
		"--pids-limit=200",
		"-v", absProfile+":/profile",
		"-e", "DISPLAY_NUM=99",
		"-e", "VNC_PORT=5900",
		"-e", "CDP_PORT=9222",
		"-e", "PROFILE_DIR=/profile",
		// Labels for PortRegistry recovery after server restart
		fmt.Sprintf("--label=thg.account_id=%d", accountID),
		fmt.Sprintf("--label=thg.cdp_port=%d", cdpPort),
		fmt.Sprintf("--label=thg.vnc_port=%d", vncPort),
		m.dockerImage,
	)

	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		if m.portRegistry != nil {
			m.portRegistry.Release(accountID)
		}
		return nil, fmt.Errorf(
			"docker run failed: %w\n→ Is Docker installed? Is the image built? Run: docker build -t thg-browser ./docker/",
			err,
		)
	}

	fullID := strings.TrimSpace(string(out))
	shortID := fullID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	// If we used random ports (no registry), query them from Docker.
	if cdpPort == 0 || vncPort == 0 {
		vncPort, err = m.queryContainerPort(containerName, "5900")
		if err != nil {
			exec.Command("docker", "rm", "-f", containerName).Run() //nolint:errcheck
			return nil, fmt.Errorf("get container VNC port: %w", err)
		}
		cdpPort, err = m.queryContainerPort(containerName, "9222")
		if err != nil {
			log.Printf("[Workspace] WARNING: CDP port query failed for account %d: %v (screen proxy will retry)", accountID, err)
		}
	}

	inst := &Instance{
		AccountID:   accountID,
		AccountName: accountName,
		ProfileDir:  profileDir,
		ContainerID: shortID,
		CDPPort:     cdpPort,
		VNCPort:     vncPort,
		StartedAt:   time.Now(),
	}
	m.instances[accountID] = inst

	log.Printf("[Workspace] Container started for account %d (%s) — id=%s vnc=127.0.0.1:%d",
		accountID, accountName, shortID, vncPort)
	return inst, nil
}

// Stop kills and removes the Docker container for accountID.
func (m *Manager) Stop(accountID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	containerName := fmt.Sprintf("%s%d", containerPrefix, accountID)
	exec.Command("docker", "stop", "-t", "5", containerName).Run() //nolint:errcheck
	exec.Command("docker", "rm", containerName).Run()               //nolint:errcheck
	delete(m.instances, accountID)
	if m.portRegistry != nil {
		m.portRegistry.Release(accountID)
	}
	log.Printf("[Workspace] Container stopped for account %d", accountID)
}

// StopAll stops all tracked containers (called on server shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.instances {
		containerName := fmt.Sprintf("%s%d", containerPrefix, id)
		exec.Command("docker", "stop", "-t", "5", containerName).Run() //nolint:errcheck
		exec.Command("docker", "rm", containerName).Run()               //nolint:errcheck
		delete(m.instances, id)
	}
	log.Println("[Workspace] All browser containers stopped")
}

// ReconcileRunning scans Docker for any thg-browser-* containers that survived
// a server restart and re-adds them to the instances map.
// Call this once after NewManager() in main.go.
func (m *Manager) ReconcileRunning() {
	out, err := exec.Command("docker", "ps",
		"--filter", "name="+containerPrefix,
		"--format", "{{.Names}}",
	).Output()
	if err != nil {
		log.Printf("[Workspace] ReconcileRunning: docker ps failed: %v", err)
		return
	}

	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if !strings.HasPrefix(name, containerPrefix) || name == containerPrefix {
			continue
		}
		idStr := strings.TrimPrefix(name, containerPrefix)
		var accountID int64
		fmt.Sscanf(idStr, "%d", &accountID)
		if accountID == 0 {
			continue
		}

		vncPort, err := m.queryContainerPort(name, "5900")
		if err != nil {
			log.Printf("[Workspace] Reconcile: cannot get VNC port for %s: %v", name, err)
			continue
		}

		shortIDOut, _ := exec.Command("docker", "inspect", "--format={{slice .Id 0 12}}", name).Output()
		shortID := strings.TrimSpace(string(shortIDOut))

		cdpPort, _ := m.queryContainerPort(name, "9222")

		m.mu.Lock()
		m.instances[accountID] = &Instance{
			AccountID:   accountID,
			AccountName: name,
			ProfileDir:  m.ProfileDir(accountID),
			ContainerID: shortID,
			CDPPort:     cdpPort,
			VNCPort:     vncPort,
			StartedAt:   time.Now(),
		}
		m.mu.Unlock()

		log.Printf("[Workspace] Reconciled container for account %d (vnc=127.0.0.1:%d)", accountID, vncPort)
	}
}

// WaitForVNC blocks until the VNC port is connectable or timeout elapses.
// Returns true if VNC became ready.
func WaitForVNC(vncPort int, timeout time.Duration) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", vncPort)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// queryContainerPort asks Docker for the host-side port mapped from containerPort.
// Retries a few times because docker port may not respond immediately after docker run.
func (m *Manager) queryContainerPort(containerName, containerPort string) (int, error) {
	var lastErr error
	for i := 0; i < 10; i++ {
		out, err := exec.Command("docker", "port", containerName, containerPort).Output()
		if err == nil {
			// Output: "0.0.0.0:32768" or "127.0.0.1:32768"
			line := strings.TrimSpace(string(out))
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				var port int
				fmt.Sscanf(parts[len(parts)-1], "%d", &port)
				if port > 0 {
					return port, nil
				}
			}
			lastErr = fmt.Errorf("unexpected docker port output: %q", line)
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return 0, fmt.Errorf("docker port %s %s: %w", containerName, containerPort, lastErr)
}

// containerAlive checks whether a container is currently running.
func (m *Manager) containerAlive(containerID string) bool {
	if containerID == "" {
		return false
	}
	out, err := exec.Command(
		"docker", "inspect", "--format={{.State.Running}}", containerID,
	).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}
