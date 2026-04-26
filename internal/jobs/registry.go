package jobs

import "context"

// Handler executes a claimed job and returns the result JSON string.
// The scheduler calls Handle; the handler must NOT call the scheduler.
type Handler interface {
	Handle(ctx context.Context, job *Job) (result string, err error)
}

// Registry is a pure map from intent string → Handler.
// Register all handlers before calling Scheduler.Run.
type Registry struct {
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]Handler)}
}

// Register associates intent with a handler. Panics on duplicate registration
// to surface wiring bugs at startup rather than silently dropping a handler.
func (r *Registry) Register(intent string, h Handler) {
	if _, exists := r.handlers[intent]; exists {
		panic("jobs: duplicate handler registration for intent: " + intent)
	}
	r.handlers[intent] = h
}

// Get looks up a handler by intent. Returns false if not registered.
func (r *Registry) Get(intent string) (Handler, bool) {
	h, ok := r.handlers[intent]
	return h, ok
}
