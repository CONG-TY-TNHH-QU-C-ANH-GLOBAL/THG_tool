package events

import (
	"context"
	"log/slog"
	"sync"
)

// Sink is the optional persistence hook. When registered (typically by
// the top-level store.New at boot), every typed Info / Warn emission
// ALSO writes a row to the runtime_events table. The slog record
// remains the authoritative emission — sink failures are NEVER
// propagated to the caller and are logged at most once per failure.
//
// nil-tolerant: when no sink is registered (test binaries that don't
// open a full Store, or early-boot calls before sink registration),
// emission is slog-only.
type Sink func(ctx context.Context, level, eventName string, attrs []any)

var (
	sinkMu sync.RWMutex
	sink   Sink
)

// SetSink registers the persistence hook. Pass nil to remove. Safe for
// concurrent callers — typically called once at process boot.
func SetSink(s Sink) {
	sinkMu.Lock()
	sink = s
	sinkMu.Unlock()
}

func currentSink() Sink {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	return sink
}

// Info emits a typed runtime event at INFO level. eventName must be one
// of the constants in events.go — passing a raw string here defeats the
// taxonomy. The check_topology.sh CI gate watches for raw `"event"`
// strings outside this package.
//
// attrs are the additional slog attributes for the event. Use the Field*
// constants from events.go as keys so the dashboard sees consistent
// field names across emission sites.
//
//	events.Info(ctx, events.OutboundQueued,
//	    events.FieldOrgID,      orgID,
//	    events.FieldOutboundID, id,
//	    events.FieldActionType, msgType,
//	)
func Info(ctx context.Context, eventName string, attrs ...any) {
	if len(attrs) == 0 {
		slog.InfoContext(ctx, eventName, FieldEvent, eventName)
	} else {
		args := make([]any, 0, len(attrs)+2)
		args = append(args, FieldEvent, eventName)
		args = append(args, attrs...)
		slog.InfoContext(ctx, eventName, args...)
	}
	if s := currentSink(); s != nil {
		s(ctx, "info", eventName, attrs)
	}
}

// Warn emits a typed runtime event at WARN level. Use for failure-class
// events (hook failure, soft-fail outcomes, transient errors). Reserve
// slog.Error for unrecoverable conditions.
func Warn(ctx context.Context, eventName string, attrs ...any) {
	if len(attrs) == 0 {
		slog.WarnContext(ctx, eventName, FieldEvent, eventName)
	} else {
		args := make([]any, 0, len(attrs)+2)
		args = append(args, FieldEvent, eventName)
		args = append(args, attrs...)
		slog.WarnContext(ctx, eventName, args...)
	}
	if s := currentSink(); s != nil {
		s(ctx, "warn", eventName, attrs)
	}
}
