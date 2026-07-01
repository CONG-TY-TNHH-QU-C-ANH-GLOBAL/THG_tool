package workspace

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// closedPort returns a port number that was briefly bound then released, so
// dialing it fails with connection-refused (simulates "unreachable").
func closedPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func fakeCDPServer(t *testing.T) (*httptest.Server, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"Browser":"HeadlessChrome"}`))
	})
	mux.HandleFunc("/json/list", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"type":"page","url":"https://facebook.com/"}]`))
	})
	srv := httptest.NewServer(mux)
	_, portStr, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return srv, port
}

// TestCheck_VNCDownCDPAlive pins the 2026-07-01 behavior change: VNC
// unreachable must NOT mark the container unhealthy when CDP is alive and
// has a page open — VNC is informational only, never a restart trigger.
func TestCheck_VNCDownCDPAlive(t *testing.T) {
	cdp, cdpPort := fakeCDPServer(t)
	defer cdp.Close()

	h := NewHealthChecker()
	inst := &Instance{AccountID: 1, VNCPort: closedPort(t), CDPPort: cdpPort, StartedAt: time.Now()}

	status := h.Check(context.Background(), inst)

	if !status.Healthy {
		t.Fatalf("Healthy = false, want true (VNC-down must not gate); reason=%q", status.Reason)
	}
	if status.VNCReachable {
		t.Fatalf("VNCReachable = true, want false (port was closed)")
	}
	if !status.CDPAlive {
		t.Fatalf("CDPAlive = false, want true")
	}
	if !status.HasTabs {
		t.Fatalf("HasTabs = false, want true")
	}
	if !strings.Contains(status.Reason, "VNC port") {
		t.Fatalf("Reason = %q, want it to still mention the VNC outage informationally", status.Reason)
	}
}

// TestCheck_CDPDown pins the other half of the change: CDP unreachable IS
// the new gate — the container must be reported unhealthy regardless of
// VNC state.
func TestCheck_CDPDown(t *testing.T) {
	h := NewHealthChecker()
	inst := &Instance{AccountID: 2, VNCPort: closedPort(t), CDPPort: closedPort(t), StartedAt: time.Now()}

	status := h.Check(context.Background(), inst)

	if status.Healthy {
		t.Fatalf("Healthy = true, want false (CDP down must gate unhealthy)")
	}
	if status.CDPAlive {
		t.Fatalf("CDPAlive = true, want false")
	}
	if !strings.Contains(status.Reason, "CDP") {
		t.Fatalf("Reason = %q, want it to mention the CDP failure", status.Reason)
	}
}
