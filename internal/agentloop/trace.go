package agentloop

import (
	"sync"
	"time"
)

// DecisionTrace records every LLM decision and executor result in order.
// It is append-only within one agent run. Thread-safe.
type DecisionTrace struct {
	mu      sync.Mutex
	traceID string
	entries []TraceEntry
}

func newDecisionTrace(traceID string) *DecisionTrace {
	return &DecisionTrace{traceID: traceID}
}

// Record appends a step result to the trace.
func (t *DecisionTrace) Record(iteration int, step, decision, reason, result string, confidence float64, latencyMS int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, TraceEntry{
		TraceID:    t.traceID,
		Iteration:  iteration,
		Step:       step,
		Decision:   decision,
		Reason:     reason,
		Confidence: confidence,
		Result:     result,
		LatencyMS:  latencyMS,
		At:         time.Now().UTC(),
	})
}

// Entries returns a snapshot of all recorded entries.
func (t *DecisionTrace) Entries() []TraceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TraceEntry, len(t.entries))
	copy(out, t.entries)
	return out
}
