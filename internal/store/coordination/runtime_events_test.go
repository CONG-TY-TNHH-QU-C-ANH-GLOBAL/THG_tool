// Domain: coordination (see internal/store/DOMAINS.md)
package coordination_test

import (
	"context"
	"testing"
	"time"

	"github.com/thg/scraper/internal/runtime/events"
	"github.com/thg/scraper/internal/store/coordination"
)

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

	rows, err := coord.ListRecentRuntimeEvents(ctx, 7, time.Now().Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("ListRecentRuntimeEvents: %v", err)
	}
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
	rows, _ := coord.ListRecentRuntimeEvents(ctx, 0, time.Now().Add(-time.Minute), 10)
	if len(rows) != 1 || rows[0].Level != "info" {
		t.Fatalf("expected level=info default, got %+v", rows)
	}
}

// TestRecordRuntimeEvent_OrgScoping pins the read scoping rule: a
// non-superadmin query (orgID > 0) returns rows for that org plus
// system-wide (org_id=0) events; never another org's rows.
func TestRecordRuntimeEvent_OrgScoping(t *testing.T) {
	_, coord := newCoordinationStore(t, "runtime_events_org.db")
	ctx := context.Background()

	_ = coord.RecordRuntimeEvent(ctx, "info", events.OutboundQueued, []any{events.FieldOrgID, int64(1)})
	_ = coord.RecordRuntimeEvent(ctx, "info", events.OutboundQueued, []any{events.FieldOrgID, int64(2)})
	_ = coord.RecordRuntimeEvent(ctx, "info", events.EngagementReconcile, []any{events.FieldOrgID, int64(0)})

	rows, err := coord.ListRecentRuntimeEvents(ctx, 1, time.Now().Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	gotByOrg := map[int64]int{}
	for _, r := range rows {
		gotByOrg[r.OrgID]++
	}
	if gotByOrg[1] != 1 {
		t.Errorf("expected 1 row for org=1, got %d", gotByOrg[1])
	}
	if gotByOrg[0] != 1 {
		t.Errorf("expected 1 system-wide (org=0) row visible, got %d", gotByOrg[0])
	}
	if gotByOrg[2] != 0 {
		t.Errorf("org=2 row leaked into org=1 view: %d", gotByOrg[2])
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

	rows, err := coord.ListRecentRuntimeEvents(ctx, 99, time.Now().Add(-time.Minute), 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
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
