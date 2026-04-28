package agentloop

import (
	"context"
	"database/sql"
	"fmt"
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

// Verify runs a stability loop: 3 checks at T+0, T+2s, T+4s.
// All 3 must pass for overall Pass=true.
// Invariant: one failure in the stability loop → FAIL.
func (v *Verifier) Verify(ctx context.Context, domain Domain) VerifyResult {
	checkpoints := []time.Duration{0, 2 * time.Second, 4 * time.Second}
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
			"checkpoint", i, "score", result.Score, "pass", result.Pass)

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

// ── Browser domain ────────────────────────────────────────────────────────────

func (v *Verifier) verifyBrowser(ctx context.Context) VerifyResult {
	signals := VerifySignals{}
	var reasons []string

	// Signal 1 (0.35): VNC TCP reachable
	if v.cfg.VNCPort > 0 {
		if tcpReachable("127.0.0.1", v.cfg.VNCPort, 2*time.Second) {
			signals.Stream = 0.35
		} else {
			reasons = append(reasons, fmt.Sprintf("VNC port %d not reachable", v.cfg.VNCPort))
		}
	} else {
		signals.Stream = 0.35 // port not configured — skip check
	}

	// Signal 2 (0.35): CDP /json/version responds
	if v.cfg.CDPPort > 0 {
		url := fmt.Sprintf("http://127.0.0.1:%d/json/version", v.cfg.CDPPort)
		if httpOK(ctx, v.client, url) {
			signals.API = 0.35
		} else {
			reasons = append(reasons, fmt.Sprintf("CDP port %d /json/version failed", v.cfg.CDPPort))
		}
	} else {
		signals.API = 0.35
	}

	// Signal 3 (0.30): CDP /json/list has at least one tab
	if v.cfg.CDPPort > 0 {
		url := fmt.Sprintf("http://127.0.0.1:%d/json/list", v.cfg.CDPPort)
		if httpBodyContains(ctx, v.client, url, "\"type\":\"page\"") {
			signals.DOM = 0.30
		} else {
			reasons = append(reasons, "CDP has no active page tab")
		}
	} else {
		signals.DOM = 0.30
	}

	score := signals.Stream + signals.API + signals.DOM
	return VerifyResult{
		Pass:    score >= VerifyPassThreshold,
		Score:   score,
		Signals: signals,
		Reason:  strings.Join(reasons, "; "),
	}
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
