package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/thg/scraper/internal/runtime/events"
)

// TestEventConstantsAreStable pins the wire format. Adding a constant
// here = a new event in the dashboard taxonomy. Renaming one = a
// breaking change to every dashboard query — don't do it lightly.
func TestEventConstantsAreStable(t *testing.T) {
	cases := map[string]string{
		events.CrawlerURLRepair:      "crawler.url_repair",
		events.CrawlerIngestSkip:     "crawler.ingest_skip",
		events.OutboundQueued:        "outbound.queued",
		events.OutboundClaimed:       "outbound.claimed",
		events.OutboundFinalized:     "outbound.finalized",
		events.OutboundQueueRejected: "outbound.queue_rejected",
		events.ExecutionAttemptBegun: "execution.attempt_begun",
		events.ExecutionVerified:     "execution.verified",
		events.ExecutionHookFailed:   "execution.hook_failed",
		events.EngagementReconcile:   "engagement.reconcile",
		events.EngagementRevoked:     "engagement.revoked",
		events.RiskSignalApplied:     "risk.signal_applied",
		events.RiskCooldownSet:       "risk.cooldown_set",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("constant value drift: got %q, want %q", got, want)
		}
	}
}

// TestInfoEmitsTypedField verifies the typed `event` field is the
// first attribute in the slog record, so dashboard queries can rely on
// position-stable JSON keys.
func TestInfoEmitsTypedField(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	events.Info(context.Background(), events.OutboundQueued,
		events.FieldOrgID, int64(7),
		events.FieldOutboundID, int64(42),
	)

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("emit produced non-JSON: %v\n%s", err, buf.String())
	}
	if rec["event"] != "outbound.queued" {
		t.Errorf("event field = %v, want outbound.queued", rec["event"])
	}
	if rec["org_id"] != float64(7) {
		t.Errorf("org_id field = %v, want 7", rec["org_id"])
	}
	if rec["outbound_id"] != float64(42) {
		t.Errorf("outbound_id field = %v, want 42", rec["outbound_id"])
	}
}

// TestWarnUsesWarnLevel so failure-class events route to the warn
// pipeline without an extra parameter on every call site.
func TestWarnUsesWarnLevel(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	events.Warn(context.Background(), events.ExecutionHookFailed,
		events.FieldHook, "RecordLedgerTx",
		events.FieldErr, "tx already closed",
	)

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("emit produced non-JSON: %v\n%s", err, buf.String())
	}
	if rec["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", rec["level"])
	}
	if rec["hook"] != "RecordLedgerTx" {
		t.Errorf("hook field = %v, want RecordLedgerTx", rec["hook"])
	}
}

// TestEmitWithNoAttrs handles the boundary case where a caller fires a
// pure event without context fields.
func TestEmitWithNoAttrs(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))

	events.Info(context.Background(), events.EngagementReconcile)

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("emit produced non-JSON: %v\n%s", err, buf.String())
	}
	if rec["event"] != "engagement.reconcile" {
		t.Errorf("event field = %v, want engagement.reconcile", rec["event"])
	}
}
