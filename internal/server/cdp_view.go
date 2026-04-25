package server

// cdp_view.go — Per-account real-time browser view via Chrome DevTools Protocol screencast.
//
// Architecture:
//   Chrome (CDP, one per account) ← cdpViewHub → WebSocket clients (dashboard Browser tab)
//   Client mouse/keyboard events → WebSocket → cdpViewHub → CDP input dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	fiberws "github.com/gofiber/websocket/v2"
)

// cdpViewHub manages all dashboard WebSocket clients watching ONE account's browser.
type cdpViewHub struct {
	mu     sync.RWMutex
	subs   map[*fiberws.Conn]bool
	cdpCtx context.Context
	cancel context.CancelFunc
}

func newCDPViewHub() *cdpViewHub {
	return &cdpViewHub{subs: make(map[*fiberws.Conn]bool)}
}

func (h *cdpViewHub) subscribe(ws *fiberws.Conn) {
	h.mu.Lock()
	h.subs[ws] = true
	h.mu.Unlock()
}

func (h *cdpViewHub) unsubscribe(ws *fiberws.Conn) {
	h.mu.Lock()
	delete(h.subs, ws)
	h.mu.Unlock()
}

func (h *cdpViewHub) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ws := range h.subs {
		_ = ws.WriteMessage(fiberws.TextMessage, data)
	}
}

func (h *cdpViewHub) subscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// getAccountHub returns the hub for accountID, or nil if none exists.
func (s *Server) getAccountHub(accountID int64) *cdpViewHub {
	s.cdpHubsMu.RLock()
	defer s.cdpHubsMu.RUnlock()
	return s.cdpHubs[accountID]
}

// getOrCreateAccountHub returns the hub for accountID, creating one if needed.
func (s *Server) getOrCreateAccountHub(accountID int64) *cdpViewHub {
	s.cdpHubsMu.Lock()
	defer s.cdpHubsMu.Unlock()
	if h, ok := s.cdpHubs[accountID]; ok {
		return h
	}
	h := newCDPViewHub()
	s.cdpHubs[accountID] = h
	return h
}

// startAccountScreencast connects to the Chrome instance for accountID (via cdpPort)
// and begins streaming JPEG frames to all subscribed WebSocket clients.
func (s *Server) startAccountScreencast(accountID int64, cdpPort int) {
	hub := s.getOrCreateAccountHub(accountID)

	hub.mu.Lock()
	// Stop existing screencast if any
	if hub.cancel != nil {
		hub.cancel()
		hub.cdpCtx = nil
		hub.cancel = nil
	}
	hub.mu.Unlock()

	wsURL, err := waitForChromeWS(cdpPort)
	if err != nil {
		log.Printf("[CDPView] Account %d: Chrome not ready on port %d: %v", accountID, cdpPort, err)
		return
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	// WithTargetID not used — connect to the first existing tab (Chrome's default tab)
	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	cancelAll := func() { ctxCancel(); allocCancel() }

	hub.mu.Lock()
	hub.cdpCtx = ctx
	hub.cancel = cancelAll
	hub.mu.Unlock()

	// Enable the Page domain so CDP sends screencast events.
	// We do NOT auto-navigate — the user drives Chrome themselves via the canvas.
	if err := chromedp.Run(ctx, page.Enable()); err != nil {
		log.Printf("[CDPView] Account %d: page.Enable error: %v", accountID, err)
		cancelAll()
		return
	}

	// Forward screencast frames to all subscribed WebSocket clients
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if e, ok := ev.(*page.EventScreencastFrame); ok {
			msg, _ := json.Marshal(map[string]any{
				"type":         "frame",
				"data":         e.Data,
				"sessionID":    e.SessionID,
				"deviceWidth":  e.Metadata.DeviceWidth,
				"deviceHeight": e.Metadata.DeviceHeight,
			})
			hub.broadcast(msg)
			go func(sid int64) {
				_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
					return page.ScreencastFrameAck(sid).Do(c)
				}))
			}(e.SessionID)
		}
	})

	if err := chromedp.Run(ctx, page.StartScreencast().
		WithFormat(page.ScreencastFormatJpeg).
		WithQuality(80).
		WithMaxWidth(1280).
		WithMaxHeight(800).
		WithEveryNthFrame(1),
	); err != nil {
		log.Printf("[CDPView] Account %d: startScreencast error: %v", accountID, err)
		cancelAll()
		return
	}
	log.Printf("[CDPView] Screencast started for account %d (port %d)", accountID, cdpPort)

	<-ctx.Done()
	log.Printf("[CDPView] Screencast stopped for account %d", accountID)
}

// stopAccountScreencast disconnects the CDP screencast for accountID.
func (s *Server) stopAccountScreencast(accountID int64) {
	hub := s.getAccountHub(accountID)
	if hub == nil {
		return
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.cancel != nil {
		hub.cancel()
		hub.cancel = nil
		hub.cdpCtx = nil
	}
}

// cdpViewHandler returns a WebSocket handler that streams JPEG frames for accountID.
// GET /ws/browser-view/:id
func (s *Server) cdpViewHandler() func(*fiberws.Conn) {
	return func(ws *fiberws.Conn) {
		idStr := ws.Params("id")
		var accountID int64
		fmt.Sscanf(idStr, "%d", &accountID)

		hub := s.getOrCreateAccountHub(accountID)
		hub.subscribe(ws)
		defer hub.unsubscribe(ws)

		log.Printf("[CDPView] Dashboard connected to account %d view", accountID)

		for {
			_, raw, err := ws.ReadMessage()
			if err != nil {
				break
			}
			var evt map[string]any
			if json.Unmarshal(raw, &evt) == nil {
				s.dispatchCDPInput(accountID, evt)
			}
		}
		log.Printf("[CDPView] Dashboard disconnected from account %d view", accountID)
	}
}

// dispatchCDPInput forwards mouse/keyboard events from the dashboard to Chrome via CDP.
func (s *Server) dispatchCDPInput(accountID int64, evt map[string]any) {
	hub := s.getAccountHub(accountID)
	if hub == nil {
		return
	}
	hub.mu.RLock()
	ctx := hub.cdpCtx
	hub.mu.RUnlock()
	if ctx == nil {
		return
	}

	evtType, _ := evt["type"].(string)
	x, _ := evt["x"].(float64)
	y, _ := evt["y"].(float64)

	switch evtType {
	case "mousemove":
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(c)
		}))
	case "mousedown":
		btn := cdpMouseButton(evt["button"])
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(btn).WithClickCount(1).Do(c)
		}))
	case "mouseup":
		btn := cdpMouseButton(evt["button"])
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(btn).WithClickCount(1).Do(c)
		}))
	case "wheel":
		dx, _ := evt["deltaX"].(float64)
		dy, _ := evt["deltaY"].(float64)
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseWheel, x, y).WithDeltaX(dx).WithDeltaY(dy).Do(c)
		}))
	case "keydown":
		k, _ := evt["key"].(string)
		code, _ := evt["code"].(string)
		mods := int64(cdpModifiers(evt))
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchKeyEvent(input.KeyDown).WithKey(k).WithCode(code).WithModifiers(input.Modifier(mods)).Do(c)
		}))
		if len(k) == 1 {
			_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
				return input.DispatchKeyEvent(input.KeyChar).WithText(k).Do(c)
			}))
		}
	case "keyup":
		k, _ := evt["key"].(string)
		code, _ := evt["code"].(string)
		mods := int64(cdpModifiers(evt))
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchKeyEvent(input.KeyUp).WithKey(k).WithCode(code).WithModifiers(input.Modifier(mods)).Do(c)
		}))
	}
}

// waitForChromeWS polls the Chrome CDP endpoint until it responds (up to 10s).
func waitForChromeWS(cdpPort int) (string, error) {
	for i := 0; i < 10; i++ {
		wsURL, err := chromeBrowserWS(cdpPort)
		if err == nil {
			return wsURL, nil
		}
		time.Sleep(1 * time.Second)
	}
	return "", fmt.Errorf("chrome not ready on port %d", cdpPort)
}

func cdpMouseButton(v any) input.MouseButton {
	switch fmt.Sprint(v) {
	case "1", "middle":
		return input.Middle
	case "2", "right":
		return input.Right
	}
	return input.Left
}

func cdpModifiers(evt map[string]any) int {
	m := 0
	if b, _ := evt["altKey"].(bool); b {
		m |= 1
	}
	if b, _ := evt["ctrlKey"].(bool); b {
		m |= 2
	}
	if b, _ := evt["metaKey"].(bool); b {
		m |= 4
	}
	if b, _ := evt["shiftKey"].(bool); b {
		m |= 8
	}
	return m
}
