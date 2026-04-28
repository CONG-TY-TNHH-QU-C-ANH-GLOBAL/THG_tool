package agentloop

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Verifier checks whether the system goal is met after patches are applied.
// It is domain-aware: each domain runs a different set of checks.
//
// Invariant GOAL_BASED_SUCCESS: Pass = goal achieved, not "no errors".
// Invariant NO_FAKE_SUCCESS: build OK alone is NOT sufficient.
type Verifier struct {
	cfg    VerifyConfig
	client *http.Client
}

// NewVerifier creates a Verifier with the given domain configuration.
func NewVerifier(cfg VerifyConfig) *Verifier {
	return &Verifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// stabilityCheckpoints returns the timing for the stability loop by domain.
//
// Browser sessions require a longer soak: Facebook's anti-bot checks,
// lazy-loading, and goroutine startup can cause false passes at T+4s that
// become failures at T+30s.
//
// Other domains are lower-risk; a shorter loop avoids unnecessary latency.
func stabilityCheckpoints(domain Domain) []time.Duration {
	if domain == DomainBrowser {
		return []time.Duration{0, 5 * time.Second, 15 * time.Second, 30 * time.Second}
	}
	return []time.Duration{0, 2 * time.Second, 4 * time.Second}
}

// Verify runs a domain-appropriate stability loop.
// Browser: T+0, T+5s, T+15s, T+30s.  Others: T+0, T+2s, T+4s.
// All checkpoints must pass for overall Pass=true.
// Invariant: one failure in the stability loop → FAIL.
func (v *Verifier) Verify(ctx context.Context, domain Domain) VerifyResult {
	checkpoints := stabilityCheckpoints(domain)
	var lastResult VerifyResult

	for i, delay := range checkpoints {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return VerifyResult{Reason: "context cancelled during stability loop"}
			case <-time.After(delay):
			}
		}

		result := v.checkOnce(ctx, domain)
		slog.DebugContext(ctx, "verifier stability check",
			"checkpoint", i, "delay_s", int(delay.Seconds()),
			"score", result.Score, "pass", result.Pass)

		if !result.Pass {
			result.Reason = fmt.Sprintf("stability loop failed at T+%ds: %s", int(delay.Seconds()), result.Reason)
			return result
		}
		lastResult = result
	}
	return lastResult
}

// checkOnce runs all domain-specific checks once and returns a scored result.
func (v *Verifier) checkOnce(ctx context.Context, domain Domain) VerifyResult {
	switch domain {
	case DomainBrowser:
		return v.verifyBrowser(ctx)
	case DomainFrontend:
		return v.verifyFrontend(ctx)
	case DomainInfra:
		return v.verifyInfra(ctx)
	case DomainJob:
		return v.verifyJob(ctx)
	default:
		// Unknown domain: lightweight HTTP health check only.
		return v.verifyGeneric(ctx)
	}
}

// ── Browser domain (FB business-aware) ───────────────────────────────────────
//
// 5-signal model — infra health + Facebook session validity:
//
//	Signal 1 (0.15): VNC TCP reachable         — container running
//	Signal 2 (0.15): CDP /json/version responds — Chrome alive
//	Signal 3 (0.15): CDP has ≥1 page tab       — Chrome has a tab open
//	Signal 4 (0.35): Facebook session alive     — not checkpoint/login page (CRITICAL)
//	Signal 5 (0.20): Facebook target page ready — correct URL context
//
// Invariant GOAL_BASED_SUCCESS: score ≥ 0.70 requires at least Signal 4
// (FB session alive). Infra-only pass is impossible.

func (v *Verifier) verifyBrowser(ctx context.Context) VerifyResult {
	signals := VerifySignals{}
	var reasons []string

	// Signal 1 (0.15): VNC TCP reachable
	if v.cfg.VNCPort > 0 {
		if tcpReachable("127.0.0.1", v.cfg.VNCPort, 2*time.Second) {
			signals.Stream = 0.15
		} else {
			reasons = append(reasons, fmt.Sprintf("VNC port %d unreachable", v.cfg.VNCPort))
		}
	} else {
		signals.Stream = 0.15 // not configured — skip
	}

	// Signal 2 (0.15): CDP /json/version responds
	var cdpAlive bool
	if v.cfg.CDPPort > 0 {
		versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", v.cfg.CDPPort)
		if httpOK(ctx, v.client, versionURL) {
			signals.API = 0.15
			cdpAlive = true
		} else {
			reasons = append(reasons, fmt.Sprintf("CDP port %d /json/version failed", v.cfg.CDPPort))
		}
	} else {
		signals.API = 0.15
		cdpAlive = true // not configured — assume up
	}

	if !cdpAlive {
		// CDP is down — Signals 3, 4, 5 are impossible
		return VerifyResult{
			Pass:    false,
			Score:   signals.Stream + signals.API,
			Signals: signals,
			Reason:  strings.Join(reasons, "; "),
		}
	}

	// Signal 3 (0.15): CDP has ≥1 page tab — quick HTTP check before WebSocket eval.
	tabs := v.fetchCDPTabs(ctx)
	hasPageTab := false
	for _, t := range tabs {
		if t.Type == "page" {
			hasPageTab = true
			break
		}
	}
	if hasPageTab {
		signals.DOM = 0.15
	} else {
		reasons = append(reasons, "CDP has no page tab open")
	}

	// Signals 4 + 5: real Facebook session via CDP WebSocket JS evaluation.
	// CDPSessionChecker connects to the live tab and evaluates JS to read DOM state —
	// stronger than URL parsing because it catches expired cookies and overlays.
	var fbScore float64
	if v.cfg.CDPPort > 0 {
		cdpChecker := NewCDPSessionChecker(v.cfg.CDPPort)
		state, cdpErr := cdpChecker.Check(ctx)
		if cdpErr != nil {
			reasons = append(reasons, "CDP session eval failed: "+cdpErr.Error())
		} else {
			ok, reason := state.IsSessionHealthy(v.cfg.ExpectedFBUserID)
			if ok {
				// Session alive + determine if on a target page (Signal 5 = +0.20)
				if strings.Contains(state.URL, "/groups/") ||
					strings.Contains(state.URL, "/marketplace/") ||
					strings.Contains(state.URL, "/home.php") ||
					strings.Contains(state.URL, "/feed/") ||
					strings.Contains(state.URL, "/profile.php") ||
					strings.Contains(state.URL, "/messages/") {
					fbScore = 0.55 // Signal 4 (0.35) + Signal 5 (0.20)
				} else {
					fbScore = 0.35 // Signal 4 only — session alive but not on target page
				}
			} else {
				reasons = append(reasons, reason)
			}
		}
	} else {
		// CDP port not configured — fall back to HTTP tab URL heuristic.
		var fbReasons []string
		fbScore, fbReasons = v.checkFacebookSession(tabs)
		reasons = append(reasons, fbReasons...)
	}
	signals.Stream += fbScore

	score := signals.Stream + signals.API + signals.DOM
	return VerifyResult{
		Pass:    score >= VerifyPassThreshold,
		Score:   min1(score),
		Signals: signals,
		Reason:  strings.Join(reasons, "; "),
	}
}

// fbTab is one entry from CDP /json/list.
type fbTab struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// fetchCDPTabs calls CDP HTTP /json/list and returns parsed tabs.
// Returns empty slice on any error (caller checks len).
func (v *Verifier) fetchCDPTabs(ctx context.Context) []fbTab {
	if v.cfg.CDPPort <= 0 {
		return nil
	}
	listURL := fmt.Sprintf("http://127.0.0.1:%d/json/list", v.cfg.CDPPort)
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var tabs []fbTab
	_ = json.Unmarshal(body, &tabs)
	return tabs
}

// checkFacebookSession analyses CDP tab URLs for Facebook-specific session signals.
// Returns (score, failureReasons).
//
// Score breakdown:
//
//	0.55 = logged-in Facebook tab is on a feed/group/target page (Signal 4 + 5)
//	0.35 = logged in to Facebook (any page, not checkpoint/login) (Signal 4 only)
//	0.00 = checkpoint detected, logged out, or no FB tab
func (v *Verifier) checkFacebookSession(tabs []fbTab) (float64, []string) {
	var reasons []string

	for _, tab := range tabs {
		if tab.Type != "page" {
			continue
		}
		u := strings.ToLower(tab.URL)

		if !strings.Contains(u, "facebook.com") {
			continue // not a Facebook tab
		}

		// ⚠️ Ban signal: checkpoint page — session blocked by Facebook
		if strings.Contains(u, "checkpoint") || strings.Contains(u, "/checkpoint/") {
			return 0, []string{"CRITICAL: Facebook checkpoint detected — account requires human verification"}
		}

		// ⚠️ Logged out
		if strings.Contains(u, "facebook.com/login") ||
			strings.Contains(u, "facebook.com/r.php") ||
			strings.Contains(u, "facebook.com/?next=") {
			return 0, []string{"Facebook login page detected — session expired or logged out"}
		}

		// ⚠️ Anti-bot soft block
		if strings.Contains(u, "/sorry/") || strings.Contains(u, "checkpoint") ||
			strings.Contains(tab.Title, "You're Temporarily Blocked") ||
			strings.Contains(tab.Title, "Blocked") {
			return 0, []string{"Facebook anti-bot block detected in tab title/URL"}
		}

		// ✅ Session alive — check if we're on the right target page (Signal 5)
		targetPages := []string{
			"/groups/", "/marketplace/", "/home.php", "facebook.com/",
			"/feed/", "/profile.php", "/messages/",
		}
		for _, tgt := range targetPages {
			if strings.Contains(u, tgt) {
				return 0.55, nil // Signal 4 (0.35) + Signal 5 (0.20)
			}
		}

		// Logged in but not on target page
		return 0.35, nil // Signal 4 only — session alive but wrong page context
	}

	reasons = append(reasons, "no active Facebook tab found in browser")
	return 0, reasons
}

func min1(f float64) float64 {
	if f > 1.0 {
		return 1.0
	}
	return f
}

// ── Frontend domain ───────────────────────────────────────────────────────────

func (v *Verifier) verifyFrontend(ctx context.Context) VerifyResult {
	signals := VerifySignals{}
	var reasons []string

	url := v.cfg.FrontendURL
	if url == "" {
		url = "http://localhost:3000"
	}

	// Signal 1 (0.50): HTTP 200
	if httpOK(ctx, v.client, url) {
		signals.HTTP = 0.50
	} else {
		reasons = append(reasons, "frontend not returning HTTP 200")
	}

	// Signal 2 (0.50): No "Application error" in body (Next.js error page)
	if !httpBodyContains(ctx, v.client, url, "Application error") &&
		!httpBodyContains(ctx, v.client, url, "Internal Server Error") {
		signals.DOM = 0.50
	} else {
		reasons = append(reasons, "frontend response contains error page content")
	}

	score := signals.HTTP + signals.DOM
	return VerifyResult{
		Pass:    score >= VerifyPassThreshold,
		Score:   score,
		Signals: signals,
		Reason:  strings.Join(reasons, "; "),
	}
}

// ── Infra domain ──────────────────────────────────────────────────────────────

func (v *Verifier) verifyInfra(ctx context.Context) VerifyResult {
	signals := VerifySignals{}
	var reasons []string

	// Signal 1 (0.40): container is running
	if v.cfg.ContainerName != "" {
		if containerRunning(ctx, v.cfg.ContainerName) {
			signals.Container = 0.40
		} else {
			reasons = append(reasons, fmt.Sprintf("container %q is not running", v.cfg.ContainerName))
		}
	} else {
		signals.Container = 0.40 // not configured — skip
	}

	// Signal 2 (0.30): nginx / API health endpoint
	if v.cfg.FrontendURL != "" {
		if httpOK(ctx, v.client, v.cfg.FrontendURL+"/health") ||
			httpOK(ctx, v.client, v.cfg.FrontendURL) {
			signals.API = 0.30
		} else {
			reasons = append(reasons, "backend health check failed")
		}
	} else {
		signals.API = 0.30
	}

	// Signal 3 (0.30): container logs do not contain crash markers in last 20 lines
	if v.cfg.ContainerName != "" {
		if !containerLogContains(ctx, v.cfg.ContainerName, []string{"panic:", "fatal error:", "OOM"}) {
			signals.Stream = 0.30
		} else {
			reasons = append(reasons, "container logs contain crash signal")
		}
	} else {
		signals.Stream = 0.30
	}

	score := signals.Container + signals.API + signals.Stream
	return VerifyResult{
		Pass:    score >= VerifyPassThreshold,
		Score:   score,
		Signals: signals,
		Reason:  strings.Join(reasons, "; "),
	}
}

// ── Job domain ────────────────────────────────────────────────────────────────

func (v *Verifier) verifyJob(ctx context.Context) VerifyResult {
	signals := VerifySignals{}
	var reasons []string

	// Signal 1 (0.60): no jobs stuck in "running" for > 10 minutes
	if v.cfg.JobDBPath != "" {
		stuck, err := countStuckJobs(ctx, v.cfg.JobDBPath)
		if err != nil {
			reasons = append(reasons, "cannot query job DB: "+err.Error())
		} else if stuck > 0 {
			reasons = append(reasons, fmt.Sprintf("%d jobs stuck in running state", stuck))
		} else {
			signals.Job = 0.60
		}
	} else {
		signals.Job = 0.60
	}

	// Signal 2 (0.40): API health (backend is up)
	if v.cfg.FrontendURL != "" {
		if httpOK(ctx, v.client, v.cfg.FrontendURL+"/api/stats") {
			signals.API = 0.40
		} else {
			reasons = append(reasons, "job API not reachable")
		}
	} else {
		signals.API = 0.40
	}

	score := signals.Job + signals.API
	return VerifyResult{
		Pass:    score >= VerifyPassThreshold,
		Score:   score,
		Signals: signals,
		Reason:  strings.Join(reasons, "; "),
	}
}

// ── Generic fallback ──────────────────────────────────────────────────────────

func (v *Verifier) verifyGeneric(ctx context.Context) VerifyResult {
	if v.cfg.FrontendURL == "" {
		return VerifyResult{
			Pass:   true,
			Score:  1.0,
			Reason: "no verification target configured — treating as pass",
		}
	}
	ok := httpOK(ctx, v.client, v.cfg.FrontendURL)
	score := 0.0
	reason := "HTTP check failed"
	if ok {
		score = 1.0
		reason = ""
	}
	return VerifyResult{Pass: ok, Score: score, Reason: reason}
}

// ── Low-level helpers ─────────────────────────────────────────────────────────

func tcpReachable(host string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func httpOK(ctx context.Context, client *http.Client, url string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

func httpBodyContains(ctx context.Context, client *http.Client, url, substr string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	buf := make([]byte, 8192)
	n, _ := resp.Body.Read(buf)
	return strings.Contains(string(buf[:n]), substr)
}

func containerRunning(ctx context.Context, name string) bool {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--filter",
		"name="+name, "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), name)
}

func containerLogContains(ctx context.Context, name string, markers []string) bool {
	out, err := exec.CommandContext(ctx, "docker", "logs", "--tail", "20", name).CombinedOutput()
	if err != nil {
		return false
	}
	body := string(out)
	for _, m := range markers {
		if strings.Contains(body, m) {
			return true
		}
	}
	return false
}

func countStuckJobs(ctx context.Context, dbPath string) (int, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var count int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scheduler_jobs
		 WHERE status='running' AND started_at < datetime('now','-10 minutes')`).Scan(&count)
	return count, err
}
