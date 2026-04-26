package identity

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
)

// BrowserFingerprint is a stable per-account browser identity.
// Values are derived deterministically from accountID so they survive restarts.
type BrowserFingerprint struct {
	AccountID  int64  `json:"account_id"`
	UserAgent  string `json:"user_agent"`
	Platform   string `json:"platform"`
	Languages  string `json:"languages"`
	ScreenW    int    `json:"screen_w"`
	ScreenH    int    `json:"screen_h"`
	ColorDepth int    `json:"color_depth"`
	Timezone   string `json:"timezone"`
	WebGLVendor string `json:"webgl_vendor"`
	WebGLRenderer string `json:"webgl_renderer"`
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
}

var screens = [][2]int{
	{1920, 1080}, {1366, 768}, {1440, 900}, {1536, 864}, {1280, 720},
}

var timezones = []string{
	"Asia/Ho_Chi_Minh", "Asia/Bangkok", "Asia/Singapore", "Asia/Jakarta",
}

var webglVendors = []string{
	"Google Inc. (NVIDIA)", "Google Inc. (Intel)", "Google Inc. (AMD)",
}

var webglRenderers = []string{
	"ANGLE (NVIDIA, NVIDIA GeForce GTX 1660 Direct3D11 vs_5_0 ps_5_0, D3D11)",
	"ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)",
	"ANGLE (AMD, AMD Radeon RX 580 Direct3D11 vs_5_0 ps_5_0, D3D11)",
}

// Generate returns a stable deterministic fingerprint for the given accountID.
// Same accountID always produces same fingerprint.
func Generate(accountID int64) BrowserFingerprint {
	h := sha256.Sum256([]byte(fmt.Sprintf("fp:account:%d", accountID)))
	seed := int64(h[0])<<56 | int64(h[1])<<48 | int64(h[2])<<40 | int64(h[3])<<32 |
		int64(h[4])<<24 | int64(h[5])<<16 | int64(h[6])<<8 | int64(h[7])
	r := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic, not crypto

	sc := screens[r.Intn(len(screens))]
	return BrowserFingerprint{
		AccountID:     accountID,
		UserAgent:     userAgents[r.Intn(len(userAgents))],
		Platform:      "Win32",
		Languages:     "vi-VN,vi,en-US,en",
		ScreenW:       sc[0],
		ScreenH:       sc[1],
		ColorDepth:    24,
		Timezone:      timezones[r.Intn(len(timezones))],
		WebGLVendor:   webglVendors[r.Intn(len(webglVendors))],
		WebGLRenderer: webglRenderers[r.Intn(len(webglRenderers))],
	}
}

// Manager caches fingerprints in memory (they're deterministic, so no DB needed).
type Manager struct {
	cache map[int64]BrowserFingerprint
}

func NewManager() *Manager {
	return &Manager{cache: make(map[int64]BrowserFingerprint)}
}

func (m *Manager) Get(accountID int64) BrowserFingerprint {
	if fp, ok := m.cache[accountID]; ok {
		return fp
	}
	fp := Generate(accountID)
	m.cache[accountID] = fp
	return fp
}
