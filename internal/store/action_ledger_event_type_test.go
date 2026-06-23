// Domain: coordination (append-only ledger PR1 foundation; see
// specs/store/APPEND_ONLY_LEDGER_MIGRATION.md). Characterization tests proving
// migration 0023 is additive-only: it changes no reader/writer behavior, the
// new column defaults to 'action_attempted', and pre-existing rows back-fill.
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
)

// TestActionLedgerEventType_FreshMigrationAndWriterUnchanged verifies that a
// fresh DB applies the 0023 migration, the existing outbound→ledger writer path
// is UNCHANGED, and a newly-written row gets event_type defaulted WITHOUT any
// writer code mentioning the column.
func TestActionLedgerEventType_FreshMigrationAndWriterUnchanged(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "ledger_event_type.db"))
	if err != nil {
		t.Fatalf("New (fresh migration chain incl. 0023): %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	if !actionLedgerColumnExists(t, db.db, "event_type") {
		t.Fatal("0023 did not add action_ledger.event_type on a fresh DB")
	}

	// Existing writer path unchanged: queueing an outbound still inserts a
	// ledger row with the legacy outcome 'queued' AND the new column defaulted
	// to 'action_attempted' (the writer SQL never names event_type).
	res, err := db.QueueOutboundForOrg(&models.OutboundMessage{
		OrgID: 1, Type: "comment", Platform: "facebook",
		AccountID: 10, TargetURL: "https://facebook.com/groups/1/posts/100", Content: "x",
	}, 24*time.Hour)
	if err != nil || !res.Decision.Allowed {
		t.Fatalf("queue outbound: err=%v decision=%+v", err, res.Decision)
	}

	var eventType, outcome string
	if err := db.db.QueryRowContext(ctx,
		`SELECT event_type, outcome FROM action_ledger ORDER BY id DESC LIMIT 1`,
	).Scan(&eventType, &outcome); err != nil {
		t.Fatalf("read ledger row: %v", err)
	}
	if eventType != "action_attempted" {
		t.Fatalf("new row event_type = %q, want 'action_attempted' (additive default)", eventType)
	}
	if outcome != "queued" {
		t.Fatalf("legacy outcome behavior changed: outcome = %q, want 'queued'", outcome)
	}
}

// TestActionLedgerEventType_BackfillsExistingRows proves the 0023 ALTER
// back-fills PRE-EXISTING rows (written before the column existed) to the
// default, and does not mutate any other column — the additive-safety
// guarantee for legacy data.
func TestActionLedgerEventType_BackfillsExistingRows(t *testing.T) {
	raw, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "backfill.db"))
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer raw.Close()

	// Pre-0023 action_ledger shape with a populated legacy row.
	if _, err := raw.Exec(`CREATE TABLE action_ledger (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER NOT NULL,
		action_type TEXT NOT NULL,
		target_url TEXT NOT NULL,
		outbound_id INTEGER NOT NULL DEFAULT 0,
		performed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		outcome TEXT NOT NULL DEFAULT 'queued'
	)`); err != nil {
		t.Fatalf("create legacy action_ledger: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO action_ledger (org_id, action_type, target_url, outbound_id, outcome)
		 VALUES (1, 'comment', 'https://facebook.com/p/1', 5, 'succeeded')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	// Apply the EXACT additive statements from migration 0023.
	if _, err := raw.Exec(
		`ALTER TABLE action_ledger ADD COLUMN event_type TEXT NOT NULL DEFAULT 'action_attempted'`); err != nil {
		t.Fatalf("ALTER add event_type: %v", err)
	}
	if _, err := raw.Exec(
		`CREATE INDEX IF NOT EXISTS idx_action_ledger_event_outbound
			ON action_ledger(outbound_id, event_type, performed_at DESC) WHERE outbound_id > 0`); err != nil {
		t.Fatalf("create partial index: %v", err)
	}

	var eventType, outcome string
	if err := raw.QueryRow(
		`SELECT event_type, outcome FROM action_ledger WHERE id = 1`).Scan(&eventType, &outcome); err != nil {
		t.Fatalf("read back-filled row: %v", err)
	}
	if eventType != "action_attempted" {
		t.Fatalf("existing row event_type = %q, want 'action_attempted' (back-fill)", eventType)
	}
	if outcome != "succeeded" {
		t.Fatalf("existing row outcome mutated to %q, want 'succeeded' (additive must not touch data)", outcome)
	}
}

func actionLedgerColumnExists(t *testing.T, db *sql.DB, col string) bool {
	t.Helper()

	rows, err := db.Query("PRAGMA table_info(action_ledger)")
	if err != nil {
		t.Fatalf("pragma table_info(action_ledger): %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == col {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pragma table_info(action_ledger): %v", err)
	}
	return false
}
