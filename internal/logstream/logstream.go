package logstream

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

// Entry is one captured log line.
type Entry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Level   string    `json:"level"` // info | warn | error
}

// Hub holds a ring buffer of recent log entries and a set of live subscribers.
type Hub struct {
	mu      sync.RWMutex
	buf     []Entry
	cap     int
	subs    map[chan Entry]struct{}
	origOut io.Writer
}

var global = &Hub{
	cap:  500,
	buf:  make([]Entry, 0, 500),
	subs: make(map[chan Entry]struct{}),
}

// Global returns the process-wide log hub.
func Global() *Hub { return global }

// Install redirects the standard logger's output through the hub.
// Call once in main() after all other setup.
func Install() {
	global.origOut = log.Writer()
	log.SetOutput(global)
}

// Write implements io.Writer — captures each log line.
func (h *Hub) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n\r")
	e := Entry{
		Time:    time.Now(),
		Message: line,
		Level:   detectLevel(line),
	}

	h.mu.Lock()
	if len(h.buf) >= h.cap {
		h.buf = h.buf[1:] // drop oldest
	}
	h.buf = append(h.buf, e)
	subs := make([]chan Entry, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	// Non-blocking push to subscribers
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}

	// Mirror to original output
	if h.origOut != nil {
		_, _ = h.origOut.Write(p)
	}
	return len(p), nil
}

// Recent returns the last n log entries (or fewer if buffer is smaller).
func (h *Hub) Recent(n int) []Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n > len(h.buf) {
		n = len(h.buf)
	}
	out := make([]Entry, n)
	copy(out, h.buf[len(h.buf)-n:])
	return out
}

// Subscribe registers a channel to receive new log entries in real time.
// Call Unsubscribe when done to avoid leaks.
func (h *Hub) Subscribe() chan Entry {
	ch := make(chan Entry, 128)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (h *Hub) Unsubscribe(ch chan Entry) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
	// Drain so the writer goroutine never blocks
	for len(ch) > 0 {
		<-ch
	}
}

// SSEFormat renders an entry as a Server-Sent Event line.
func (e Entry) SSEFormat() string {
	msg := strings.ReplaceAll(e.Message, "\n", " ")
	ts := e.Time.Format("15:04:05")
	return fmt.Sprintf("data: [%s] %s\n\n", ts, msg)
}

func detectLevel(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "❌") || strings.Contains(lower, "fatal") {
		return "error"
	}
	if strings.Contains(lower, "warn") || strings.Contains(lower, "⚠") {
		return "warn"
	}
	return "info"
}
