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

		// Org-scope guard: superadmin (orgID=0) bypasses check.
		acc, err := s.db.GetAccount(id)
		if err != nil || acc == nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "account not found"})
			return
		}
		orgID, _ := ws.Locals("org_id").(int64)
		if orgID != 0 && acc.OrgID != orgID {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "access denied"})
			return
		}

		// Fetch CDP targets from Chrome inside the container.
		// Chrome takes 3-8s to start its CDP listener after x11vnc is ready,
		// so retry for up to 15s before giving up.
		targetsURL := fmt.Sprintf("http://127.0.0.1:%d/json", inst.CDPPort)
		var (
			resp *http.Response
			body []byte
		)
		for attempt := 0; attempt < 15; attempt++ {
			resp, err = http.Get(targetsURL) //nolint:noctx
			if err == nil {
				body, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
				break
			}
			time.Sleep(time.Second)
		}
		if err != nil {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "CDP unreachable sau 15s: " + err.Error()})
			return
		}

		var targets []struct {
			WS   string `json:"webSocketDebuggerUrl"`
			Type string `json:"type"`
		}
		_ = json.Unmarshal(body, &targets)

		var cdpWSURL string
		for _, t := range targets {
			if t.Type == "page" {
				cdpWSURL = t.WS
				break
			}
		}
		if cdpWSURL == "" {
			_ = ws.WriteJSON(screenFrame{Type: "error", Msg: "no Chrome page target found"})
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
		send := func(method string, params map[string]any) {
			mid := atomic.AddInt64(&msgID, 1)
			_ = cdp.WriteJSON(map[string]any{"id": mid, "method": method, "params": params})
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
				send("Page.screencastFrameAck", map[string]any{"sessionId": int(sid)})
				if err := ws.WriteJSON(screenFrame{Type: "frame", Data: data, W: 1280, H: 800}); err != nil {
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
					send("Input.dispatchMouseEvent", map[string]any{
						"type":       inp.Action,
						"x":          inp.X,
						"y":          inp.Y,
						"button":     inp.Button,
						"buttons":    inp.Buttons,
						"clickCount": 1,
						"modifiers":  inp.Modifiers,
					})
				case "wheel":
					send("Input.dispatchMouseEvent", map[string]any{
						"type":   "mouseWheel",
						"x":      inp.X,
						"y":      inp.Y,
						"deltaX": inp.DeltaX,
						"deltaY": inp.DeltaY,
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
