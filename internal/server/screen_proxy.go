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
	"sync"
	"sync/atomic"
	"time"

	fiberws "github.com/gofiber/websocket/v2"
	gorillaWS "github.com/gorilla/websocket"
	"github.com/thg/scraper/internal/models"
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

// screenProxyHandler streams Chrome desktop as JPEG frames via CDP Page.startScreencast
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

		// Org-scope guard: superadmin (orgID=0) bypasses check.
		acc, err := s.db.GetAccount(id)
		if err != nil || acc == nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "account not found"})
			return
		}
		orgID, _ := ws.Locals("org_id").(int64)
		role, _ := ws.Locals("user_role").(string)
		if !models.IsPlatformUser(orgID, models.UserRole(role)) && acc.OrgID != orgID {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "access denied"})
			return
		}

		// Fetch CDP targets from Chrome inside the container.
		// Chrome can take longer on small production hosts after Docker launches;
		// even when /json responds it may briefly return no page targets.
		// Retry for up to 90s, sending a status ping every 3s so the client
		// knows the connection is alive during Chrome's startup window.
		targetsURL := fmt.Sprintf("http://127.0.0.1:%d/json", inst.CDPPort)
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
			ctx5s, cancel5s := context.WithTimeout(context.Background(), 5*time.Second)
			req, _ := http.NewRequestWithContext(ctx5s, http.MethodGet, targetsURL, nil)
			resp, httpErr := http.DefaultClient.Do(req)
			cancel5s()
			if httpErr != nil {
				err = httpErr
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			err = nil

			var targets []struct {
				ID   string `json:"id"`
				WS   string `json:"webSocketDebuggerUrl"`
				Type string `json:"type"`
			}
			_ = json.Unmarshal(body, &targets)
			for _, t := range targets {
				if t.Type == "page" {
					cdpWSURL = t.WS
					targetID = t.ID
					break
				}
			}
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
					button := inp.Button
					if button == "" {
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
				}
			}
		}()

		<-ctx.Done()
		log.Printf("[Screen] account %d session closed", id)
	}
}
