package agent

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	fiberws "github.com/gofiber/websocket/v2"
	"github.com/thg/scraper/internal/store"
)

// ExtClient represents a single connected Chrome Extension instance.
type ExtClient struct {
	conn    *fiberws.Conn
	agentID int64
	name    string
	send    chan []byte
}

// WSHub manages all connected Chrome Extension WebSocket clients.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*ExtClient]bool
}

// NewWSHub creates a ready-to-use WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{clients: make(map[*ExtClient]bool)}
}

// ConnectedCount returns the number of currently connected extension clients.
func (h *WSHub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// NotifyOutboxReady broadcasts that there are approved messages to send.
func (h *WSHub) NotifyOutboxReady(count int) {
	h.broadcast(map[string]any{"type": "outbox_ready", "count": count})
}

func (h *WSHub) broadcast(msg any) {
	data, _ := json.Marshal(msg)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			// client buffer full — drop silently
		}
	}
}

func (h *WSHub) register(c *ExtClient) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
}

func (h *WSHub) deregister(c *ExtClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// wsHandler returns a Fiber-compatible WebSocket handler authenticated via agent token.
func (h *WSHub) WSHandler(db *store.Store) func(*fiberws.Conn) {
	return func(c *fiberws.Conn) {
		// Step 1: first message must be auth
		var authMsg struct {
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
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		if err := c.ReadJSON(&authMsg); err != nil || authMsg.Type != "auth" || authMsg.Token == "" {
			_ = c.WriteJSON(map[string]string{"type": "error", "message": "auth required"})
			return
		}
		_ = c.SetReadDeadline(time.Time{})

		tok, err := db.ValidateAgentToken(authMsg.Token)
		if err != nil || tok == nil {
			_ = c.WriteJSON(map[string]string{"type": "error", "message": "invalid token"})
			return
		}

		client := &ExtClient{
			conn:    c,
			agentID: tok.ID,
			name:    tok.Name,
			send:    make(chan []byte, 64),
		}

		h.register(client)
		defer h.deregister(client)

		_ = db.UpdateAgentPresence(tok.ID, store.AgentPresence{
			Hostname:          authMsg.Hostname,
			OS:                authMsg.OS,
			Version:           authMsg.Version,
			Kind:              authMsg.Kind,
			Transport:         authMsg.Transport,
			AssignedAccountID: authMsg.AccountID,
			CapabilitiesJSON:  authMsg.CapabilitiesJSON,
			CurrentURL:        authMsg.CurrentURL,
			FBUserID:          authMsg.FBUserID,
			StreamStatus:      authMsg.StreamStatus,
		})
		log.Printf("[WSHub] Extension %q connected (id=%d), total=%d", client.name, client.agentID, h.ConnectedCount())

		// Send welcome so extension knows auth succeeded
		_ = c.WriteJSON(map[string]any{
			"type":     "welcome",
			"agent_id": tok.ID,
			"name":     tok.Name,
		})

		// Write pump: pushes queued messages to the WebSocket
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case data, ok := <-client.send:
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
		}()

		// Read pump: handles pong/status messages from extension
		for {
			_, raw, err := c.ReadMessage()
			if err != nil {
				break
			}
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				switch t, _ := m["type"].(string); t {
				case "pong":
					_ = db.UpdateAgentHeartbeat(tok.ID, "", "", "")
				case "status":
					p := store.AgentPresence{}
					if v, ok := m["current_url"].(string); ok {
						p.CurrentURL = v
					}
					if v, ok := m["fb_user_id"].(string); ok {
						p.FBUserID = v
					}
					if v, ok := m["stream_status"].(string); ok {
						p.StreamStatus = v
					}
					_ = db.UpdateAgentPresence(tok.ID, p)
				}
			}
		}

		<-done
		log.Printf("[WSHub] Extension %q disconnected, total=%d", client.name, h.ConnectedCount())
	}
}
