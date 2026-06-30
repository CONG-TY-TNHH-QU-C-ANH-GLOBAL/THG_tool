// Domain: coordination (see internal/store/DOMAINS.md)
package coordination_test

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store/coordination"
)

// recentRuntimeEvents reads back persisted rows via raw SQL (the typed
// ListRecentRuntimeEvents reader was removed as dead production code when the
// runtime-feed dashboard handler was deleted). Tests here exercise the LIVE
// write path (RecordRuntimeEvent + the boot sink); this helper is just their
// read-back assertion tool, newest-first.
func recentRuntimeEvents(t *testing.T, coord *coordination.Store, limit int) []coordination.RuntimeEvent {
	t.Helper()
	rows, err := coord.DB().QueryContext(context.Background(),
		`SELECT id, org_id, account_id, event, level, outbound_id, attempt_id, target_url, attrs_json
		   FROM runtime_events ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		t.Fatalf("read runtime_events: %v", err)
	}
	defer rows.Close()
	var out []coordination.RuntimeEvent
	for rows.Next() {
		var r coordination.RuntimeEvent
		if err := rows.Scan(&r.ID, &r.OrgID, &r.AccountID, &r.Event, &r.Level,
			&r.OutboundID, &r.AttemptID, &r.TargetURL, &r.AttrsJSON); err != nil {
			t.Fatalf("scan runtime_event: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

// TestRecordRuntimeEvent_RoundTrip pins the structural-field extraction
// from the slog attribute stream. Known keys land in typed columns;
// the rest collapse into attrs_json.
func TestRecordRuntimeEvent_RoundTrip(t *testing.T) {
	_, coord := newCoordinationStore(t, "runtime_events_roundtrip.db")
	ctx := context.Background()

	err := coord.RecordRuntimeEvent(ctx, "info", events.OutboundQueued, []any{
		events.FieldOrgID, int64(7),
		events.FieldAccountID, int64(42),
		events.FieldOutboundID, int64(1001),
		events.FieldActionType, "comment",
		"cooldown", "24h0m0s",
	})
	if err != nil {
		t.Fatalf("RecordRuntimeEvent: %v", err)
	}

	rows := recentRuntimeEvents(t, coord, 10)
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.Event != events.OutboundQueued {
		t.Errorf("event = %q, want %q", r.Event, events.OutboundQueued)
	}
	if r.OrgID != 7 {
		t.Errorf("org_id = %d, want 7 (peeled from attrs)", r.OrgID)
	}
	if r.AccountID != 42 {
		t.Errorf("account_id = %d, want 42", r.AccountID)
	}
	if r.OutboundID != 1001 {
		t.Errorf("outbound_id = %d, want 1001", r.OutboundID)
	}
	if r.Level != "info" {
		t.Errorf("level = %q, want info", r.Level)
	}
	if r.AttrsJSON == "" || r.AttrsJSON == "{}" {
		t.Errorf("attrs_json should contain remainder (action_type, cooldown); got %q", r.AttrsJSON)
	}
}

// TestRecordRuntimeEvent_LevelDefault validates that an empty level
// string defaults to "info" rather than persisting as blank.
func TestRecordRuntimeEvent_LevelDefault(t *testing.T) {
	_, coord := newCoordinationStore(t, "runtime_events_level.db")
	ctx := context.Background()

	if err := coord.RecordRuntimeEvent(ctx, "", events.EngagementReconcile, nil); err != nil {
		t.Fatalf("record: %v", err)
	}
	rows := recentRuntimeEvents(t, coord, 10)
	if len(rows) != 1 || rows[0].Level != "info" {
		t.Fatalf("expected level=info default, got %+v", rows)
	}
}

// TestSinkDualWrite verifies the boot-time wiring: an events.Info call
// goes to slog AND lands in runtime_events when the sink is registered.
func TestSinkDualWrite(t *testing.T) {
	_, coord := newCoordinationStore(t, "runtime_events_sink.db")
	ctx := context.Background()

	// Register the sink as the parent store would at boot.
	events.SetSink(func(ctx context.Context, level, eventName string, attrs []any) {
		_ = coord.RecordRuntimeEvent(ctx, level, eventName, attrs)
	})
	t.Cleanup(func() { events.SetSink(nil) })

	events.Info(ctx, events.RiskSignalApplied,
		events.FieldOrgID, int64(99),
		events.FieldAccountID, int64(123),
		"signal", "RiskSignalFailure",
	)

	rows := recentRuntimeEvents(t, coord, 10)
	if len(rows) != 1 {
		t.Fatalf("dual-write did not persist: got %d rows", len(rows))
	}
	if rows[0].Event != events.RiskSignalApplied {
		t.Errorf("event = %q, want %q", rows[0].Event, events.RiskSignalApplied)
	}
}

// Compile-time guard: the parent store wires installRuntimeEventSink
// as Sink type. If the signature drifts, this test breaks fast.
var _ events.Sink = func(ctx context.Context, level, eventName string, attrs []any) {
	_ = (*coordination.Store)(nil).RecordRuntimeEvent(ctx, level, eventName, attrs)
}
