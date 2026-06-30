package stream

import (
	"encoding/json"
	"testing"
	"time"
)

// Characterization tests for the WebSocket hub (ws_hub.go). These PIN the
// CURRENT behavior of the hub registry, the non-blocking broadcast/backpressure
// drop, the deregister channel-close, and the outbox_ready payload shape — so a
// later behavior-preserving complexity refactor of WSHandler (PR22B) has a safety
// net. They are white-box (package agent) and exercise only the hub methods that
// do NOT touch the *fiberws.Conn or *store.Store: register, deregister, broadcast,
// ConnectedCount, NotifyOutboxReady. WSHandler's auth/welcome/ping/read-write-pump
// paths require a live WebSocket connection and are NOT covered here (documented
// gap — see PR22A report).
//
// No production code is touched. No time.Sleep, no ticker timing, no polling: the
// only timeout is a bounded deadlock guard that proves a call returned.

// wsProdSendBuffer mirrors the per-client send-channel capacity WSHandler uses in
// production (`make(chan []byte, 64)`). Tests construct clients with this size so
// the backpressure/drop characterization matches the real buffer.
const wsProdSendBuffer = 64

// wsCharDeadline bounds the "did the call return / did a message arrive" waits.
// It is a DEADLOCK GUARD only — success is the channel signal, never the timeout.
const wsCharDeadline = 2 * time.Second

// newWSTestClient builds an ExtClient with only the fields the hub registry,
// broadcast, and deregister paths read (the send channel). conn/db are never
// dereferenced by those paths, so they are left nil.
func newWSTestClient(capacity int) *ExtClient {
	return &ExtClient{send: make(chan []byte, capacity)}
}

// requireConnectedCount asserts the hub's connected count via the public,
// lock-safe ConnectedCount() accessor (never reads h.clients directly).
func requireConnectedCount(t *testing.T, h *WSHub, want int) {
	t.Helper()
	if got := h.ConnectedCount(); got != want {
		t.Fatalf("ConnectedCount() = %d, want %d", got, want)
	}
}

// readWSMessage reads exactly one queued message, using a bounded deadlock guard
// so a missing/late enqueue fails fast instead of hanging the suite.
func readWSMessage(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case data, ok := <-ch:
		if !ok {
			t.Fatal("send channel closed; expected a queued message")
		}
		return data
	case <-time.After(wsCharDeadline):
		t.Fatal("timed out waiting for a broadcast message (deadlock guard)")
		return nil
	}
}

// TestWSHubRegisterDeregisterLifecycle pins: fresh hub starts at 0, register adds
// one, deregister removes it AND closes the client's send channel, and a second
// deregister is a no-op (the `if _, ok := h.clients[c]; ok` guard) that neither
// panics nor double-closes.
func TestWSHubRegisterDeregisterLifecycle(t *testing.T) {
	h := NewWSHub()
	requireConnectedCount(t, h, 0)

	client := newWSTestClient(wsProdSendBuffer)
	h.register(client)
	requireConnectedCount(t, h, 1)

	h.deregister(client)
	requireConnectedCount(t, h, 0)

	// deregister closes the send channel: a receive on the now-empty closed
	// channel returns immediately with ok=false (deterministic, no blocking).
	if _, ok := <-client.send; ok {
		t.Fatal("expected send channel closed after deregister")
	}

	// Second deregister must be a safe no-op (idempotent guard) — no panic, no
	// double-close, count stays 0.
	h.deregister(client)
	requireConnectedCount(t, h, 0)
}

// TestWSHubMultipleClients pins map membership + per-client cleanup: two distinct
// clients count as 2, and removing them one at a time decrements correctly.
func TestWSHubMultipleClients(t *testing.T) {
	h := NewWSHub()
	a := newWSTestClient(wsProdSendBuffer)
	b := newWSTestClient(wsProdSendBuffer)

	h.register(a)
	h.register(b)
	requireConnectedCount(t, h, 2)

	h.deregister(a)
	requireConnectedCount(t, h, 1)

	h.deregister(b)
	requireConnectedCount(t, h, 0)
}

// TestWSHubNotifyOutboxReadyPayload pins the outbox_ready broadcast shape exactly:
// it enqueues one JSON message carrying only {"type":"outbox_ready","count":N}.
func TestWSHubNotifyOutboxReadyPayload(t *testing.T) {
	h := NewWSHub()
	client := newWSTestClient(wsProdSendBuffer)
	h.register(client)

	h.NotifyOutboxReady(7)

	var m map[string]any
	if err := json.Unmarshal(readWSMessage(t, client.send), &m); err != nil {
		t.Fatalf("outbox_ready payload is not valid JSON: %v", err)
	}
	if m["type"] != "outbox_ready" {
		t.Fatalf(`payload["type"] = %v, want "outbox_ready"`, m["type"])
	}
	// JSON numbers decode to float64 through map[string]any.
	if m["count"] != float64(7) {
		t.Fatalf(`payload["count"] = %v, want 7`, m["count"])
	}
	if len(m) != 2 {
		t.Fatalf("payload has %d keys %v, want exactly 2 (type,count)", len(m), m)
	}
}

// TestWSHubBroadcastBackpressureDrops pins the non-blocking drop: when a
// registered client's send buffer is FULL, broadcast must return without blocking
// and silently drop the message (the `select { case c.send <- data: default: }`
// path), leaving the buffer untouched.
func TestWSHubBroadcastBackpressureDrops(t *testing.T) {
	h := NewWSHub()
	client := newWSTestClient(wsProdSendBuffer)
	h.register(client)

	// Saturate the buffer deterministically.
	for i := 0; i < wsProdSendBuffer; i++ {
		client.send <- []byte("queued")
	}
	if len(client.send) != wsProdSendBuffer {
		t.Fatalf("precondition: send len = %d, want full %d", len(client.send), wsProdSendBuffer)
	}

	// broadcast must NOT block on the full buffer. Success = the call returns
	// (done closes); the timeout is only a deadlock guard.
	done := make(chan struct{})
	go func() {
		h.NotifyOutboxReady(1)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(wsCharDeadline):
		t.Fatal("broadcast blocked on a full send buffer (deadlock guard)")
	}

	// The dropped message did not displace anything: buffer is still exactly full.
	if len(client.send) != wsProdSendBuffer {
		t.Fatalf("send len = %d after drop, want %d (message must drop, not block/grow)", len(client.send), wsProdSendBuffer)
	}
}

// TestWSHubBroadcastSkipsDeregisteredClient pins that a deregistered client (whose
// send channel deregister has CLOSED and removed from the registry) is not sent to
// by a subsequent broadcast — i.e. no send-on-closed-channel panic. A second, live
// client still receives, proving broadcast keeps working for remaining clients.
func TestWSHubBroadcastSkipsDeregisteredClient(t *testing.T) {
	h := NewWSHub()
	gone := newWSTestClient(wsProdSendBuffer)
	live := newWSTestClient(wsProdSendBuffer)
	h.register(gone)
	h.register(live)

	h.deregister(gone) // closes gone.send and removes it from the registry
	requireConnectedCount(t, h, 1)

	// If broadcast tried to send to the closed gone.send it would panic; it must
	// not, because gone is no longer in the registry.
	h.NotifyOutboxReady(3)

	// The remaining live client still got the message.
	var m map[string]any
	if err := json.Unmarshal(readWSMessage(t, live.send), &m); err != nil {
		t.Fatalf("live client payload not valid JSON: %v", err)
	}
	if m["type"] != "outbox_ready" {
		t.Fatalf(`live payload["type"] = %v, want "outbox_ready"`, m["type"])
	}
}
