package store

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// The PR-4 outbound version gate: an account whose connector runs an
// update_required build gets NO new outbound tasks, and the denial is
// audited as a skipped blocked_by_extension_version ledger row in the
// same transaction. A current build passes untouched.
func TestExtensionGateForOutbound(t *testing.T) {
	db := newSharedStore(t, "extension_gate.db")

	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: 1, Platform: models.PlatformFacebook, Name: "Gate FB",
		AssignedUserID: 7, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	seedConnector := func(version string) {
		if _, err := db.DB().Exec(
			`INSERT INTO agent_tokens
				(org_id, name, created_by, token_hash, kind, transport, assigned_account_id,
				 fb_user_id, stream_status, version, active, last_seen, created_at)
			 VALUES (1, 'ext', 7, ?, 'extension_connector', 'chrome_extension', ?,
			        '111', 'facebook_logged_in', ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			"hash-"+version, accID, version,
		); err != nil {
			t.Fatalf("seed connector: %v", err)
		}
	}

	gate := func() bool {
		tx, err := db.DB().Begin()
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Commit() }()
		return db.extensionGateForOutbound(tx, accID, "comment")
	}

	ledgerCount := func() int {
		var n int
		_ = db.DB().QueryRow(
			`SELECT COUNT(*) FROM action_ledger WHERE account_id = ? AND outcome = 'skipped' AND reason = ?`,
			accID, LedgerReasonExtensionBlocked,
		).Scan(&n)
		return n
	}

	// Ancient build (below the default required floor) → blocked + audited.
	seedConnector("0.5.10")
	if !gate() {
		t.Fatalf("old-extension account must be blocked")
	}
	if ledgerCount() != 1 {
		t.Fatalf("blocked queue attempt must write a skipped ledger row, got %d", ledgerCount())
	}

	// Upgrade the connector → gate opens; no new ledger rows.
	if _, err := db.DB().Exec(`UPDATE agent_tokens SET version = '0.5.54' WHERE assigned_account_id = ?`, accID); err != nil {
		t.Fatalf("upgrade connector: %v", err)
	}
	if gate() {
		t.Fatalf("current-extension account must pass the gate")
	}
	if ledgerCount() != 1 {
		t.Fatalf("passing gate must not add ledger rows, got %d", ledgerCount())
	}
}
