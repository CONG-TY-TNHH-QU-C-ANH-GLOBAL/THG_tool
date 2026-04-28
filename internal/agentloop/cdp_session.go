package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// CDPSessionChecker evaluates the real Facebook session state by connecting
// to a CDP WebSocket and executing JavaScript in the live browser tab.
//
// This is strictly stronger than HTTP URL polling (/json/list) because it:
//   - Reads the actual DOM (not just the cached tab URL)
//   - Detects expired cookies even on a visually correct-looking page
//   - Detects silent bans / soft-block overlays
//   - Verifies the Facebook feed is truly interactive, not a shell
//
// Invariant: only one evaluate call per check to minimise detection surface.
type CDPSessionChecker struct {
	cdpPort int
	client  *http.Client
}

// FBSessionState is the JS-evaluated state of the active Facebook tab.
type FBSessionState struct {
	URL           string `json:"url"`
	Title         string `json:"title"`
	HasFeed       bool   `json:"has_feed"`       // [role="feed"] present
	HasCheckpoint bool   `json:"has_checkpoint"` // "checkpoint" in body text
	IsLoginPage   bool   `json:"is_login_page"`  // /login or r.php in URL
	IsBlocked     bool   `json:"is_blocked"`     // "blocked" or "banned" in title
	CookieCount   int    `json:"cookie_count"`   // number of cookies set
	UserID        string `json:"user_id"`        // c_user cookie value — FB numeric user ID
}

// NewCDPSessionChecker creates a checker for the given CDP port.
func NewCDPSessionChecker(cdpPort int) *CDPSessionChecker {
	return &CDPSessionChecker{
		cdpPort: cdpPort,
		client:  &http.Client{Timeout: 4 * time.Second},
	}
}

// sessionEvalJS is the expression evaluated inside the browser tab.
// Kept as a single-line string to fit in one CDP Runtime.evaluate call.
// user_id extracts the Facebook numeric user ID from the c_user cookie —
// the same identifier Facebook's own code relies on for session ownership.
const sessionEvalJS = `(function(){` +
	`var u=window.location.href;` +
	`var t=document.title.toLowerCase();` +
	`var cuser='';` +
	`var cm=document.cookie.match(/(?:^|;)\s*c_user=(\d+)/);` +
	`if(cm)cuser=cm[1];` +
	`return JSON.stringify({` +
	`url:u,` +
	`title:document.title,` +
	`has_feed:!!document.querySelector('[role="feed"]'),` +
	`has_checkpoint:document.body.innerText.toLowerCase().indexOf('checkpoint')>=0,` +
	`is_login_page:u.indexOf('/login')>=0||u.indexOf('r.php')>=0,` +
	`is_blocked:t.indexOf('blocked')>=0||t.indexOf('banned')>=0,` +
	`cookie_count:document.cookie.split(';').filter(function(c){return c.trim()!==''}).length,` +
	`user_id:cuser` +
	`});})()`

// Check finds the active Facebook tab, connects to its CDP WebSocket,
// evaluates the session JS, and returns the parsed state.
// Returns (nil, err) if CDP is unreachable or no Facebook tab found.
func (c *CDPSessionChecker) Check(ctx context.Context) (*FBSessionState, error) {
	tab, err := c.findFBTab(ctx)
	if err != nil {
		return nil, err
	}

	state, err := c.evalInTab(ctx, tab.WSDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("cdp eval: %w", err)
	}
	return state, nil
}

// ── types ─────────────────────────────────────────────────────────────────────

type cdpTab struct {
	Type           string `json:"type"`
	URL            string `json:"url"`
	WSDebuggerURL  string `json:"webSocketDebuggerUrl"`
}

type cdpEvalRequest struct {
	ID     int            `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type cdpEvalResponse struct {
	ID     int `json:"id"`
	Result struct {
		Result struct {
			Type  string `json:"type"`
			Value any    `json:"value"`
		} `json:"result"`
	} `json:"result"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

// findFBTab returns the first page tab whose URL contains facebook.com.
func (c *CDPSessionChecker) findFBTab(ctx context.Context) (*cdpTab, error) {
	listURL := fmt.Sprintf("http://127.0.0.1:%d/json/list", c.cdpPort)
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdp /json/list unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tabs []cdpTab
	if err := json.Unmarshal(body, &tabs); err != nil {
		return nil, fmt.Errorf("cdp: parse tab list: %w", err)
	}

	for i := range tabs {
		if tabs[i].Type == "page" && strings.Contains(tabs[i].URL, "facebook.com") {
			return &tabs[i], nil
		}
	}
	return nil, fmt.Errorf("no active Facebook tab found in CDP")
}

// evalInTab opens a WebSocket to the tab's debugger URL and evaluates sessionEvalJS.
func (c *CDPSessionChecker) evalInTab(ctx context.Context, wsURL string) (*FBSessionState, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 4 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp ws connect: %w", err)
	}
	defer conn.Close()

	// Unique message ID via atomic counter (safe for single connection).
	var msgID atomic.Int32
	msgID.Store(1)
	id := int(msgID.Add(1))

	req := cdpEvalRequest{
		ID:     id,
		Method: "Runtime.evaluate",
		Params: map[string]any{
			"expression":    sessionEvalJS,
			"returnByValue": true,
		},
	}
	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("cdp ws send: %w", err)
	}

	// Read responses, ignoring events until we get our reply.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("cdp ws read: %w", err)
		}

		var raw cdpEvalResponse
		if err := json.Unmarshal(msg, &raw); err != nil || raw.ID != id {
			continue // CDP event or different response — keep waiting
		}

		// Extract the JSON string value from the Runtime.evaluate result.
		valueStr, ok := raw.Result.Result.Value.(string)
		if !ok {
			return nil, fmt.Errorf("cdp eval: unexpected result type %T", raw.Result.Result.Value)
		}

		var state FBSessionState
		if err := json.Unmarshal([]byte(valueStr), &state); err != nil {
			return nil, fmt.Errorf("cdp eval: parse JS result: %w", err)
		}
		return &state, nil
	}
}

// IsSessionHealthy returns (true, "") when the FB session is fully interactive,
// or (false, reason) when any blocking condition is detected.
//
// expectedUserID: Facebook numeric user ID (c_user cookie) that must be present.
// Pass "" to skip identity verification.
func (s *FBSessionState) IsSessionHealthy(expectedUserID string) (bool, string) {
	if s.HasCheckpoint {
		return false, "CRITICAL: Facebook checkpoint overlay detected in DOM"
	}
	if s.IsBlocked {
		return false, fmt.Sprintf("Facebook block/ban page detected (title: %q)", s.Title)
	}
	if s.IsLoginPage {
		return false, "Facebook login page in DOM — session expired or cookies cleared"
	}
	if s.CookieCount == 0 {
		return false, "no cookies present — browser profile cleared or session invalidated"
	}
	// Identity check: prevent cross-account contamination.
	// If we expected account A but account B is logged in, the verifier must fail.
	if expectedUserID != "" && s.UserID != expectedUserID {
		got := s.UserID
		if got == "" {
			got = "(none)"
		}
		return false, fmt.Sprintf("CRITICAL: wrong Facebook account — expected uid=%s, got uid=%s", expectedUserID, got)
	}
	if !s.HasFeed && !strings.Contains(s.URL, "/groups/") && !strings.Contains(s.URL, "/marketplace/") {
		return false, fmt.Sprintf("Facebook page loaded but feed element absent (url: %s)", s.URL)
	}
	return true, ""
}
