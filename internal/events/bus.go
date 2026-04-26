package events

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

// Event is the unit published to SSE subscribers.
// Follows the frontend contract: { type, payload } with optional flat fields.
type Event struct {
	Type     string          `json:"type"`               // job.created | job.running | job.progress | job.completed | job.failed | lead.inserted
	JobID    int64           `json:"job_id,omitempty"`
	TaskID   string          `json:"task_id,omitempty"`
	Status   string          `json:"status,omitempty"`
	Progress int             `json:"progress,omitempty"` // 0–100
	Message  string          `json:"message,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"` // arbitrary event-specific data
}

// Bus is an in-process pub/sub for SSE events.
// The API polls the jobs store and publishes here; SSE handlers subscribe.
type Bus struct {
	mu   sync.RWMutex
	subs map[string]chan Event
}

func NewBus() *Bus {
	return &Bus{subs: make(map[string]chan Event)}
}

// Subscribe registers a new subscriber and returns its ID and receive channel.
// The channel is buffered at 64; slow consumers drop events (non-blocking send).
func (b *Bus) Subscribe() (id string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id = uuid.New().String()
	c := make(chan Event, 64)
	b.subs[id] = c
	return id, c
}

// Unsubscribe removes the subscriber and closes its channel.
func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c, ok := b.subs[id]; ok {
		close(c)
		delete(b.subs, id)
	}
}

// Publish sends an event to all subscribers. Never blocks.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, c := range b.subs {
		select {
		case c <- e:
		default: // drop if subscriber is slow
		}
	}
}

// Len returns the number of active subscribers.
func (b *Bus) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
