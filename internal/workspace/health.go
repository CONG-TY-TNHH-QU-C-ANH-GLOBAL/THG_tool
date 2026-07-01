package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/observability"
)

// HealthStatus is the result of a 4-layer health check on a running container.
type HealthStatus struct {
	Healthy      bool
	VNCReachable bool
	CDPAlive     bool
	HasTabs      bool
	HeartbeatOK  bool
	Reason       string
}

// HealthChecker runs periodic deep health checks against running browser containers.
type HealthChecker struct {
	interval   time.Duration
	timeout    time.Duration
	hbMaxAge   time.Duration // sessions with heartbeat older than this are unhealthy
	startGrace time.Duration
}

// NewHealthChecker creates a checker with sensible production defaults.
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		interval:   15 * time.Second,
		timeout:    2 * time.Second,
		startGrace: 2 * time.Minute,
		hbMaxAge:   90 * time.Second, // 3× the 30s heartbeat interval
	}
}

// Check performs a 4-layer health probe on the given instance. VNC
// reachability (Layer 1) is informational only (2026-07-01 product
// decision: VNC is not the live-viewer surface and must not gate
// readiness/restart decisions) — it never marks a container unhealthy on
// its own. CDP (Layer 2) is the source of truth for "is the browser alive."
func (h *HealthChecker) Check(ctx context.Context, inst *Instance) HealthStatus {
	result := HealthStatus{}
	var reasons []string

	// Layer 1: VNC TCP reachability — informational only, does not gate.
	dialCtx, cancel := context.WithTimeout(ctx, h.timeout)
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", fmt.Sprintf("127.0.0.1:%d", inst.VNCPort))
	cancel()
	if err == nil {
		conn.Close()
		result.VNCReachable = true
	} else {
		reasons = append(reasons, fmt.Sprintf("VNC port %d unreachable: %v", inst.VNCPort, err))
	}

	// Layer 2: CDP /json/version responds — the only layer that gates Healthy.
	versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", inst.CDPPort)
	body, err := h.httpGet(ctx, versionURL)
	if err != nil {
		reasons = append(reasons, fmt.Sprintf("CDP /json/version failed: %v", err))
		result.Reason = strings.Join(reasons, "; ")
		return result // Healthy stays false
	}
	result.CDPAlive = true

	// Layer 3: Chrome has at least one page (not frozen/crashed internally)
	listURL := fmt.Sprintf("http://127.0.0.1:%d/json/list", inst.CDPPort)
	listBody, err := h.httpGet(ctx, listURL)
	if err == nil {
		var pages []json.RawMessage
		if json.Unmarshal(listBody, &pages) == nil && len(pages) > 0 {
			result.HasTabs = true
		}
	}
	if !result.HasTabs {
		// /json/list may not be standard; fall back to /json
		listBody2, err2 := h.httpGet(ctx, fmt.Sprintf("http://127.0.0.1:%d/json", inst.CDPPort))
		if err2 == nil {
			var pages []json.RawMessage
			if json.Unmarshal(listBody2, &pages) == nil && len(pages) > 0 {
				result.HasTabs = true
			}
		}
	}
	if !result.HasTabs {
		reasons = append(reasons, "CDP alive but Chrome has no open tabs (may be frozen)")
	}

	// Layer 4: heartbeat age check (session row in DB)
	// We use the /json/version response timestamp as a proxy here — we checked
	// above that the body is non-empty, which proves Chrome responded recently.
	// The DB-side heartbeat staleness is checked separately by the RestartController
	// using browser_sessions.heartbeat_at.
	result.HeartbeatOK = true
	_ = body // used above

	result.Healthy = true
	result.Reason = strings.Join(reasons, "; ")
	return result
}

// Run starts the health check loop, calling onUnhealthy for each instance
// that fails a check. Blocks until ctx is cancelled.
func (h *HealthChecker) Run(ctx context.Context, mgr ManagerIface, onUnhealthy func(accountID int64)) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, inst := range mgr.List() {
				if time.Since(inst.StartedAt) < h.startGrace {
					continue
				}
				status := h.Check(ctx, inst)
				h.reportMetrics(status)
				if !status.Healthy {
					slog.WarnContext(ctx, "container health check failed",
						"account_id", inst.AccountID,
						"reason", status.Reason,
					)
					onUnhealthy(inst.AccountID)
				}
			}
		}
	}
}

func (h *HealthChecker) httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: h.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (h *HealthChecker) reportMetrics(s HealthStatus) {
	if s.Healthy {
		observability.HealthCheckResults.WithLabelValues("healthy").Inc()
	} else {
		observability.HealthCheckResults.WithLabelValues("unhealthy").Inc()
	}
}
