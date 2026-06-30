package stream

import (
	"encoding/json"
	"log"
	"sync"

	fiberws "github.com/gofiber/websocket/v2"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
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
		// Step 1: first message must be auth (writes its own error + bails on failure).
		authMsg, tok, ok := authenticateWSClient(c, db)
		if !ok {
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

		_ = db.Connectors().UpdateAgentPresence(tok.ID, connectors.AgentPresence{
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

		// Write pump (own goroutine) pushes queued messages; read pump runs here
		// until the connection errors. Ordering preserved: pump starts, read loop
		// runs, we wait on done, then the deferred deregister fires.
		done := make(chan struct{})
		go runWSWritePump(c, client.send, done)
		runWSReadLoop(c, db, tok.ID)
		<-done

		log.Printf("[WSHub] Extension %q disconnected, total=%d", client.name, h.ConnectedCount())
	}
}
