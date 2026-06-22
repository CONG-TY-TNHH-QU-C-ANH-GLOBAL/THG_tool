package agent

import (
	"encoding/json"
	"time"

	fiberws "github.com/gofiber/websocket/v2"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// Per-connection WebSocket client lifecycle helpers for WSHandler (ws_hub.go).
// These were extracted verbatim from WSHandler to reduce its cognitive complexity;
// auth handshake, the write-pump goroutine body, the read loop, and per-message
// handling are unchanged — same order, conditions, payloads, and channel/goroutine
// semantics. The hub registry/broadcast methods stay in ws_hub.go.

// wsAuthMessage is the first frame an extension must send. It is the named form of
// the struct WSHandler previously declared inline; the JSON wire contract (keys
// and types) is identical.
type wsAuthMessage struct {
	Type             string `json:"type"`
	Token            string `json:"token"`
	Hostname         string `json:"hostname"`
	OS               string `json:"os"`
	Version          string `json:"version"`
	Kind             string `json:"kind"`
	Transport        string `json:"transport"`
	AccountID        int64  `json:"account_id"`
	CapabilitiesJSON string `json:"capabilities_json"`
	CurrentURL       string `json:"current_url"`
	FBUserID         string `json:"fb_user_id"`
	StreamStatus     string `json:"stream_status"`
}

// authenticateWSClient performs the Step-1 auth handshake: read the first frame
// under a 10s deadline, require type=="auth" with a token, then validate the token.
// On any failure it writes the SAME error frame WSHandler used and returns ok=false.
// On success it clears the read deadline and returns the parsed message + token.
func authenticateWSClient(c *fiberws.Conn, db *store.Store) (*wsAuthMessage, *connectors.AgentToken, bool) {
	var authMsg wsAuthMessage
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err := c.ReadJSON(&authMsg); err != nil || authMsg.Type != "auth" || authMsg.Token == "" {
		_ = c.WriteJSON(map[string]string{"type": "error", "message": "auth required"})
		return nil, nil, false
	}
	_ = c.SetReadDeadline(time.Time{})

	tok, err := db.Connectors().ValidateAgentToken(authMsg.Token)
	if err != nil || tok == nil {
		_ = c.WriteJSON(map[string]string{"type": "error", "message": "invalid token"})
		return nil, nil, false
	}
	return &authMsg, tok, true
}

// runWSWritePump is the write-pump goroutine body, extracted verbatim. It drains
// the client send channel to the socket and emits a 30s keepalive ping, exiting
// (and closing done) when the send channel is closed or a write fails. The send
// channel and done channel are passed in rather than captured; behavior — buffer,
// non-blocking drop upstream, ticker, exit conditions, close(done) timing — is
// unchanged. Started via `go runWSWritePump(...)` exactly where the goroutine
// previously started.
func runWSWritePump(c *fiberws.Conn, send <-chan []byte, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-send:
			if !ok {
				return
			}
			if err := c.WriteMessage(1, data); err != nil {
				return
			}
		case <-ticker.C:
			ping, _ := json.Marshal(map[string]string{"type": "ping"})
			if err := c.WriteMessage(1, ping); err != nil {
				return
			}
		}
	}
}

// runWSReadLoop is the handler-goroutine read loop, extracted verbatim. It reads
// frames until the connection errors (then returns), decoding each as JSON and
// dispatching it to handleWSClientMessage. Same break condition and ordering as
// the former inline loop.
func runWSReadLoop(c *fiberws.Conn, db *store.Store, agentID int64) {
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			break
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			handleWSClientMessage(db, agentID, m)
		}
	}
}

// handleWSClientMessage dispatches one decoded client frame, extracted verbatim
// from the read loop's switch: "pong" refreshes the heartbeat; "status" updates
// the agent presence from any provided current_url/fb_user_id/stream_status.
// Unknown types are ignored, as before.
func handleWSClientMessage(db *store.Store, agentID int64, m map[string]any) {
	switch t, _ := m["type"].(string); t {
	case "pong":
		_ = db.Connectors().UpdateAgentHeartbeat(agentID, "", "", "")
	case "status":
		p := connectors.AgentPresence{}
		if v, ok := m["current_url"].(string); ok {
			p.CurrentURL = v
		}
		if v, ok := m["fb_user_id"].(string); ok {
			p.FBUserID = v
		}
		if v, ok := m["stream_status"].(string); ok {
			p.StreamStatus = v
		}
		_ = db.Connectors().UpdateAgentPresence(agentID, p)
	}
}
