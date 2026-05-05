package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func probeExistingChromeStatus(port int) chromeSnapshot {
	if port <= 0 {
		port = 9222
	}
	devtoolsURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	targets, err := chromeTargets(devtoolsURL)
	if err != nil {
		return chromeSnapshot{Status: streamStatusChromeNotConnected}
	}
	status := "chrome_connected"
	var currentURL string
	for _, target := range targets {
		if target.Type != "page" {
			continue
		}
		if currentURL == "" {
			currentURL = target.URL
		}
		if strings.Contains(strings.ToLower(target.URL), "facebook.com") {
			currentURL = target.URL
			status = streamStatusFacebookLoginRequired
			break
		}
	}
	return chromeSnapshot{CurrentURL: currentURL, Status: status}
}

type chromeTargetInfo struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
	WS    string `json:"webSocketDebuggerUrl"`
}

// chromeTargetID returns t.ID, or attempts to derive it from the
// WebSocket debugger URL when the /json/list payload omitted the id
// field (some Chrome versions only emit `webSocketDebuggerUrl`).
func chromeTargetID(devtoolsURL string, t chromeTargetInfo) string {
	if id := strings.TrimSpace(t.ID); id != "" {
		return id
	}
	// webSocketDebuggerUrl looks like:
	//   ws://127.0.0.1:9260/devtools/page/<targetID>
	if t.WS == "" {
		return ""
	}
	idx := strings.LastIndex(t.WS, "/")
	if idx < 0 || idx == len(t.WS)-1 {
		return ""
	}
	return t.WS[idx+1:]
}

func chromeTargets(devtoolsURL string) ([]chromeTargetInfo, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(devtoolsURL + "/json/list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Chrome DevTools target list returned %d", resp.StatusCode)
	}
	var out []chromeTargetInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func runConnectorLoop(serverURL, token string, basePort int) {
	if basePort <= 0 {
		basePort = 9222
	}
	bridges := map[int64]*chromeBridge{}
	var bridgeMu sync.RWMutex
	lastTargetHint := ""
	lastPrintedTargetHint := ""

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	done := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() {
		stopOnce.Do(func() {
			close(done)
		})
	}
	copyBridges := func() map[int64]*chromeBridge {
		bridgeMu.RLock()
		defer bridgeMu.RUnlock()
		out := make(map[int64]*chromeBridge, len(bridges))
		for accountID, bridge := range bridges {
			out[accountID] = bridge
		}
		return out
	}
	defer func() {
		bridgeMu.Lock()
		defer bridgeMu.Unlock()
		for _, bridge := range bridges {
			if bridge != nil && bridge.cancel != nil {
				bridge.cancel()
			}
		}
	}()

	syncTargets := func() bool {
		targetResp, err := fetchBrowserTargets(serverURL, token)
		if err != nil {
			if isDeviceTokenRejected(err) {
				printDeviceTokenRejected(err)
				return false
			}
			fmt.Println("[warn] target sync failed:", err)
			return true
		}
		targets := targetResp.Targets
		if len(targets) == 0 {
			lastTargetHint = browserTargetsConsoleHint(targetResp)
			if lastTargetHint != "" && lastTargetHint != lastPrintedTargetHint {
				fmt.Println("[Target]", lastTargetHint)
				lastPrintedTargetHint = lastTargetHint
			}
		} else {
			lastTargetHint = ""
			lastPrintedTargetHint = ""
		}
		want := map[int64]browserTarget{}
		bridgeMu.Lock()
		defer bridgeMu.Unlock()
		for _, target := range targets {
			if target.AccountID <= 0 {
				continue
			}
			want[target.AccountID] = target
			if _, ok := bridges[target.AccountID]; ok {
				continue
			}
			port := localChromePort(basePort, target.AccountID)
			fmt.Printf("[Chrome] Opening %s on local port %d\n", target.AccountName, port)
			fmt.Println("[Chrome] Log in or finish Facebook checkpoint inside the Chrome window on this device.")
			fmt.Println("[Chrome] The dashboard will detect the session automatically and stream it back to Browser.")
			bridge := startChromeBridgeForTarget(target, port)
			if bridge.err != nil {
				fmt.Printf("[Chrome] %s not ready: %v\n", target.AccountName, bridge.err)
			}
			bridges[target.AccountID] = bridge
		}
		for accountID, bridge := range bridges {
			if _, ok := want[accountID]; ok {
				continue
			}
			if bridge != nil && bridge.cancel != nil {
				bridge.cancel()
			}
			delete(bridges, accountID)
		}
		return true
	}

	sendFrames := func() bool {
		best := probeExistingChromeStatus(basePort)
		current := copyBridges()
		if len(current) == 0 {
			if err := sendChromeStatus(serverURL, token, best); err != nil {
				if isDeviceTokenRejected(err) {
					printDeviceTokenRejected(err)
					return false
				}
				fmt.Println("[warn] chrome status failed:", err)
			}
		}
		for _, bridge := range current {
			snap := snapshotChrome(bridge)
			if best.AccountID == 0 || best.Status == streamStatusChromeNotConnected || snap.Status == streamStatusFacebookLoggedIn {
				best = snap
			}
			if err := sendChromeStatus(serverURL, token, snap); err != nil {
				if isDeviceTokenRejected(err) {
					printDeviceTokenRejected(err)
					return false
				}
				fmt.Printf("[warn] chrome status failed for account %d: %v\n", snap.AccountID, err)
			}
			if err := sendScreenshot(serverURL, token, snap); err != nil {
				if isDeviceTokenRejected(err) {
					printDeviceTokenRejected(err)
					return false
				}
				fmt.Printf("[warn] screenshot failed for account %d: %v\n", snap.AccountID, err)
			}
		}
		if err := sendHeartbeat(serverURL, token, best); err != nil {
			if isDeviceTokenRejected(err) {
				printDeviceTokenRejected(err)
				return false
			}
			fmt.Println("[warn] heartbeat failed:", err)
			return true
		}
		statusText := connectorConsoleStatus(best)
		if len(current) == 0 && lastTargetHint != "" {
			statusText = lastTargetHint
		}
		fmt.Printf("heartbeat ok %s - %d Chrome profile(s) - %s\n", time.Now().Format("15:04:05"), len(current), statusText)
		return true
	}

	if !syncTargets() || !sendFrames() {
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(inputPollInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !executePendingCommands(serverURL, token, copyBridges()) {
					requestStop()
					return
				}
			case <-done:
				return
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(outboxPollInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !executeApprovedOutbox(serverURL, token, copyBridges()) {
					requestStop()
					return
				}
			case <-done:
				return
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sendHeartbeat(serverURL, token, chromeSnapshot{}); err != nil {
					if isDeviceTokenRejected(err) {
						printDeviceTokenRejected(err)
						requestStop()
						return
					}
				}
			case <-done:
				return
			}
		}
	}()
	defer func() {
		requestStop()
		wg.Wait()
	}()

	frameTicker := time.NewTicker(frameInterval())
	defer frameTicker.Stop()
	targetTicker := time.NewTicker(2 * time.Second)
	defer targetTicker.Stop()
	for {
		select {
		case <-targetTicker.C:
			if !syncTargets() {
				return
			}
		case <-frameTicker.C:
			if !sendFrames() {
				return
			}
		case <-stop:
			fmt.Println()
			fmt.Println("Connector stopped.")
			return
		case <-done:
			return
		}
	}
}

func localChromePort(basePort int, accountID int64) int {
	if basePort <= 0 {
		basePort = 9222
	}
	offset := int(accountID % 10000)
	if offset < 0 {
		offset = -offset
	}
	port := basePort + offset
	if port > 65000 {
		port = 20000 + offset
	}
	return port
}

func frameInterval() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_FRAME_INTERVAL_SECONDS")))
	if seconds <= 0 {
		seconds = 2
	}
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func inputPollInterval() time.Duration {
	ms, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_INPUT_POLL_MS")))
	if ms <= 0 {
		ms = 250
	}
	if ms < 100 {
		ms = 100
	}
	if ms > 2000 {
		ms = 2000
	}
	return time.Duration(ms) * time.Millisecond
}

func outboxPollInterval() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_OUTBOX_POLL_SECONDS")))
	if seconds <= 0 {
		seconds = 5
	}
	if seconds < 2 {
		seconds = 2
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func outboundActionTimeout() time.Duration {
	seconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("THG_OUTBOUND_ACTION_TIMEOUT_SECONDS")))
	if seconds <= 0 {
		seconds = 90
	}
	if seconds < 20 {
		seconds = 20
	}
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func runHeartbeatLoop(serverURL, token string, bridge *chromeBridge) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	for {
		select {
		case <-ticker.C:
			snap := snapshotChrome(bridge)
			if err := sendHeartbeat(serverURL, token, snap); err != nil {
				if isDeviceTokenRejected(err) {
					printDeviceTokenRejected(err)
					return
				}
				fmt.Println("[warn] heartbeat failed:", err)
				continue
			}
			fmt.Printf("heartbeat ok %s - %s\n", time.Now().Format("15:04:05"), connectorConsoleStatus(snap))
		case <-stop:
			fmt.Println()
			fmt.Println("Connector stopped.")
			return
		}
	}
}

func isDeviceTokenRejected(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "device token was rejected")
}

func connectorConsoleStatus(snap chromeSnapshot) string {
	switch snap.Status {
	case streamStatusFacebookLoggedIn:
		if snap.FBUserID != "" {
			return "Facebook connected: " + snap.FBUserID
		}
		return "Facebook connected"
	case streamStatusFacebookHumanRequired:
		return "Facebook needs human verification"
	case streamStatusFacebookLoginRequired:
		return "Facebook tab is open but not logged in"
	case streamStatusChromeNotConnected:
		return "Chrome is not connected"
	default:
		return "connector online"
	}
}
