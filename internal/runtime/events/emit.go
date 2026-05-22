package events

import (
	"context"
	"log/slog"
)

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
		return
	}
	args := make([]any, 0, len(attrs)+2)
	args = append(args, FieldEvent, eventName)
	args = append(args, attrs...)
	slog.InfoContext(ctx, eventName, args...)
}

// Warn emits a typed runtime event at WARN level. Use for failure-class
// events (hook failure, soft-fail outcomes, transient errors). Reserve
// slog.Error for unrecoverable conditions.
func Warn(ctx context.Context, eventName string, attrs ...any) {
	if len(attrs) == 0 {
		slog.WarnContext(ctx, eventName, FieldEvent, eventName)
		return
	}
	args := make([]any, 0, len(attrs)+2)
	args = append(args, FieldEvent, eventName)
	args = append(args, attrs...)
	slog.WarnContext(ctx, eventName, args...)
}
