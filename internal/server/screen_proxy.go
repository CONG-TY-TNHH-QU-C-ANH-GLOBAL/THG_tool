package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	fiberws "github.com/gofiber/websocket/v2"
	gorillaWS "github.com/gorilla/websocket"
)

type screenFrame struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	W    int    `json:"w,omitempty"`
	H    int    `json:"h,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

type inputEvent struct {
	Type      string  `json:"type"`
	Action    string  `json:"action"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Button    string  `json:"button"`
	Buttons   int     `json:"buttons"`
	Key       string  `json:"key"`
	Code      string  `json:"code"`
	Modifiers int     `json:"modifiers"`
	DeltaX    float64 `json:"deltaX"`
	DeltaY    float64 `json:"deltaY"`
}

// screenProxyHandler streams a browser session as JPEG frames via CDP Page.startScreencast
// and forwards mouse/keyboard input events back to CDP Input API.
// Route: GET /ws/screen/:id  (protected by wsJWTAuth)
func (s *Server) screenProxyHandler() func(*fiberws.Conn) {
	return func(ws *fiberws.Conn) {
		id, err := strconv.ParseInt(ws.Params("id"), 10, 64)
		if err != nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "invalid account id"})
			return
		}

		if s.workspace == nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "workspace manager not available"})
			return
		}

		inst := s.workspace.Get(id)
		if inst == nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "account container not running"})
			return
		}
		if inst.CDPPort == 0 {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "CDP port not available — container may still be starting, try refreshing"})
			return
		}

		// Org-scope guard via shared helper. Superadmin (orgID=0) bypasses check.
		orgID, _ := ws.Locals("org_id").(int64)
		role, _ := ws.Locals("user_role").(string)
		if _, ok := s.requireAccountForOrgWS(orgID, role, id); !ok {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "access denied"})
			return
		}

		// Fetch CDP targets from Chrome inside the container.
		// Chrome can take longer on small production hosts after Docker launches;
		// even when /json responds it may briefly return no page targets.
		// Retry for up to 90s, sending a status ping every 3s so the client
		// knows the connection is alive during Chrome's startup window.
		var cdpWSURL string
		var targetID string
		// Immediate ping so the client knows we're working (before any delay)
		_ = ws.WriteJSON(screenFrame{Type: "status", Msg: "connecting to Chrome..."})
		for attempt := 0; attempt < 90; attempt++ {
			if attempt > 0 {
				time.Sleep(time.Second)
				// Ping the client every 3 attempts so Cloudflare/nginx don't close idle WS
				if attempt%3 == 0 {
					_ = ws.WriteJSON(screenFrame{Type: "status", Msg: fmt.Sprintf("waiting for Chrome... (%ds)", attempt)})
				}
			}
			cdpWSURL, targetID, err = resolveCDPPageWebSocket(inst.CDPPort)
			if cdpWSURL != "" {
				break
			}
		}
		if cdpWSURL == "" {
			msg := "Chrome did not respond after 90s - try Stop and Start again"
			if err != nil {
				msg = "CDP không kết nối được: " + err.Error()
			}
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: msg})
			return
		}

		// Chrome's /json endpoint returns ws://localhost:<internal-port>/...
		// That hostname resolves to the container's internal port, not the host-mapped one.
		// Rewrite the host to use the actual host-mapped CDP port so the dial succeeds.
		if u, parseErr := url.Parse(cdpWSURL); parseErr == nil {
			u.Host = fmt.Sprintf("127.0.0.1:%d", inst.CDPPort)
			cdpWSURL = u.String()
		}

		// Dial Chrome CDP WebSocket (gorilla as client dialer).
		cdp, _, err := gorillaWS.DefaultDialer.Dial(cdpWSURL, nil)
		if err != nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "CDP dial failed: " + err.Error()})
			return
		}
		defer cdp.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Atomic message ID counter for CDP commands.
		var msgID int64
		var cdpWriteMu sync.Mutex
		send := func(method string, params map[string]any) {
			mid := atomic.AddInt64(&msgID, 1)
			cdpWriteMu.Lock()
			defer cdpWriteMu.Unlock()
			_ = cdp.WriteJSON(map[string]any{"id": mid, "method": method, "params": params})
		}

		if targetID != "" {
			send("Target.activateTarget", map[string]any{"targetId": targetID})
		}
		send("Page.enable", nil)
		send("Page.startScreencast", map[string]any{
			"format":        "jpeg",
			"quality":       75,
			"maxWidth":      1280,
			"maxHeight":     800,
			"everyNthFrame": 1,
		})

		// CDP → browser: forward JPEG frames.
		go func() {
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				_, b, err := cdp.ReadMessage()
				if err != nil {
					return
				}
				var ev map[string]any
				if json.Unmarshal(b, &ev) != nil {
					continue
				}
				if ev["method"] != "Page.screencastFrame" {
					continue
				}
				params, _ := ev["params"].(map[string]any)
				if params == nil {
					continue
				}
				data, _ := params["data"].(string)
				sid, _ := params["sessionId"].(float64)
				width, height := 1280, 800
				if meta, _ := params["metadata"].(map[string]any); meta != nil {
					if v, ok := meta["deviceWidth"].(float64); ok && v > 0 {
						width = int(v)
					}
					if v, ok := meta["deviceHeight"].(float64); ok && v > 0 {
						height = int(v)
					}
				}
				send("Page.screencastFrameAck", map[string]any{"sessionId": int(sid)})
				if err := ws.WriteJSON(screenFrame{Type: "frame", Data: data, W: width, H: height}); err != nil {
					return
				}
			}
		}()

		// Browser → CDP: forward input events.
		//
		// Phase 4a defence: the FE never speaks raw CDP — every message is
		// a typed inputEvent and we project it onto a small whitelist of
		// CDP methods (Input.dispatchMouseEvent / Input.dispatchKeyEvent
		// only). The `inp.Type` switch + `allowed{Mouse,Key}Action`
		// allowlists block prompts like Action="Runtime.evaluate" or
		// Action="Debugger.enable" from being smuggled through the
		// message envelope. Anything outside the allowlist is dropped
		// silently, the operator does not get a partial-execute footgun.
		go func() {
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				_, b, err := ws.ReadMessage()
				if err != nil {
					return
				}
				var inp inputEvent
				if json.Unmarshal(b, &inp) != nil {
					continue
				}
				switch inp.Type {
				case "mouse":
					if !allowedMouseAction(inp.Action) {
						continue
					}
					button := inp.Button
					if !allowedMouseButton(button) {
						button = "none"
					}
					clickCount := 0
					if inp.Action == "mousePressed" || inp.Action == "mouseReleased" {
						clickCount = 1
					}
					send("Input.dispatchMouseEvent", map[string]any{
						"type":       inp.Action,
						"x":          inp.X,
						"y":          inp.Y,
						"button":     button,
						"buttons":    inp.Buttons,
						"clickCount": clickCount,
						"modifiers":  inp.Modifiers,
					})
				case "wheel":
					send("Input.dispatchMouseEvent", map[string]any{
						"type":      "mouseWheel",
						"x":         inp.X,
						"y":         inp.Y,
						"deltaX":    inp.DeltaX,
						"deltaY":    inp.DeltaY,
						"modifiers": inp.Modifiers,
					})
				case "key":
					if !allowedKeyAction(inp.Action) {
						continue
					}
					txt := ""
					if inp.Action == "char" {
						txt = inp.Key
					}
					send("Input.dispatchKeyEvent", map[string]any{
						"type":      inp.Action,
						"key":       inp.Key,
						"code":      inp.Code,
						"modifiers": inp.Modifiers,
						"text":      txt,
					})
				default:
					// drop unknown envelope types (e.g. attempt to inject
					// raw CDP method names through inp.Type)
					continue
				}
			}
		}()

		<-ctx.Done()
		log.Printf("[Screen] account %d session closed", id)
	}
}

// allowedMouseAction returns true when the FE-supplied mouse Action
// maps to a documented CDP Input.dispatchMouseEvent type. Unknown
// values (including malicious ones like "Runtime.evaluate") are
// rejected before reaching Chrome.
func allowedMouseAction(a string) bool {
	switch a {
	case "mousePressed", "mouseReleased", "mouseMoved":
		return true
	default:
		return false
	}
}

// allowedMouseButton restricts FE input to the CDP-documented values.
// "none" is the safe fallback for hover-style mouseMoved events.
func allowedMouseButton(b string) bool {
	switch b {
	case "none", "left", "middle", "right", "back", "forward":
		return true
	default:
		return false
	}
}

// allowedKeyAction validates the FE-supplied key event type against
// the four CDP Input.dispatchKeyEvent values.
func allowedKeyAction(a string) bool {
	switch a {
	case "keyDown", "keyUp", "rawKeyDown", "char":
		return true
	default:
		return false
	}
}

type cdpTargetInfo struct {
	ID    string `json:"id"`
	WS    string `json:"webSocketDebuggerUrl"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func resolveCDPPageWebSocket(cdpPort int) (string, string, error) {
	targets, err := fetchCDPTargets(cdpPort)
	if err != nil {
		return "", "", err
	}
	for _, t := range targets {
		if t.Type == "page" && t.WS != "" {
			return t.WS, t.ID, nil
		}
	}

	// Chrome can expose /json/version before it has a page target. Create a
	// visible Facebook tab instead of leaving the operator staring at a blank
	// screencast forever. Recent Chrome requires PUT for /json/new.
	ctx5s, cancel5s := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel5s()
	newURL := fmt.Sprintf("http://127.0.0.1:%d/json/new?%s", cdpPort, url.QueryEscape("https://www.facebook.com"))
	req, _ := http.NewRequestWithContext(ctx5s, http.MethodPut, newURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("CDP /json/new failed: HTTP %d", resp.StatusCode)
	}
	var target cdpTargetInfo
	if err := json.Unmarshal(body, &target); err != nil {
		return "", "", fmt.Errorf("CDP /json/new parse failed: %w", err)
	}
	if target.Type == "page" && target.WS != "" {
		return target.WS, target.ID, nil
	}
	return "", "", fmt.Errorf("CDP has no page target")
}

func fetchCDPTargets(cdpPort int) ([]cdpTargetInfo, error) {
	if cdpPort <= 0 {
		return nil, fmt.Errorf("CDP port not available")
	}
	return fetchCDPTargetsFromEndpoint(cdpEndpointFromPort(cdpPort))
}

func fetchCDPTargetsFromEndpoint(ep cdpEndpoint) ([]cdpTargetInfo, error) {
	if ep.BaseURL == "" {
		return nil, fmt.Errorf("CDP endpoint not available")
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	endpoints := []string{"json/list", "json"}
	deadline := time.Now().Add(8 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		for _, endpoint := range endpoints {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			targetsURL := strings.TrimRight(ep.BaseURL, "/") + "/" + endpoint
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, targetsURL, nil)
			resp, err := client.Do(req)
			if err != nil {
				cancel()
				lastErr = err
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			cancel()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				lastErr = fmt.Errorf("CDP /%s HTTP %d", endpoint, resp.StatusCode)
				continue
			}
			var targets []cdpTargetInfo
			if err := json.Unmarshal(body, &targets); err != nil {
				lastErr = fmt.Errorf("CDP /%s parse failed: %w", endpoint, err)
				continue
			}
			return targets, nil
		}
		time.Sleep(350 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("CDP target list timed out")
	}
	return nil, lastErr
}
