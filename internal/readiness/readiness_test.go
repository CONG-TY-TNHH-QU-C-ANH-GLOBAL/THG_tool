package readiness

import (
	"context"
	"fmt"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapReadinessStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func TestEvaluateCrawlAccountReadiness_RejectsAndReasons(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapReadinessStore, "crawl_readiness")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(5)

	// 1. account_id=0 → account_not_selected (NO silent fallback to a ready account).
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", 0); r != ReasonAccountNotSelected {
		t.Fatalf("account_id=0 → want %q, got %q", ReasonAccountNotSelected, r)
	}

	// 2. Non-existent account → account_not_owned.
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", 9999); r != ReasonAccountNotOwned {
		t.Fatalf("nonexistent account → want %q, got %q", ReasonAccountNotOwned, r)
	}

	// Seed an owned account with no connector.
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "acc-a", Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	// 3. Owned account, no online connector → connector_offline.
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", accID); r != ReasonConnectorOffline {
		t.Fatalf("no connector → want %q, got %q", ReasonConnectorOffline, r)
	}

	// 4. Actor-mismatch-blocked account → actor_mismatch_blocked (takes precedence).
	if err := db.Coordination().RecordAccountActorVerdict(ctx, orgID, accID,
		models.ActorVerdictMismatch, "222", "mismatch", true); err != nil {
		t.Fatalf("RecordAccountActorVerdict: %v", err)
	}
	if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", accID); r != ReasonActorMismatchBlocked {
		t.Fatalf("blocked account → want %q, got %q", ReasonActorMismatchBlocked, r)
	}
}

// TestEvaluateCrawlAccountReadiness_OwnershipGate pins the ownership preflight
// (readiness.go: `userID > 0 && !IsAccountOwnerAllowed`). It is a SECURITY gate —
// a sales member must not preflight-pass another member's account — and it must be
// checked BEFORE the connector probe, so a non-owner is rejected as account_not_owned
// even when the connector is offline. Owner/admin/legacy paths fall through to the
// connector check (connector_offline here, since no connector is seeded), proving the
// gate fires for non-owners ONLY and that admin override + the legacy userID=0 bypass
// are preserved.
func TestEvaluateCrawlAccountReadiness_OwnershipGate(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapReadinessStore, "crawl_readiness_ownership")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID, owner int64 = 5, 7

	// Account owned by user 7, no connector → ownership decides before the connector probe.
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "owned", Status: models.AccountActive, AssignedUserID: owner,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	cases := []struct {
		name   string
		userID int64
		role   string
		want   string
	}{
		// SECURITY: non-owner sales member is blocked, AND ownership is checked before the
		// connector probe (offline connector would otherwise yield connector_offline).
		{"non-owner sales blocked", 8, "sales", ReasonAccountNotOwned},
		// Owner passes ownership → falls through to the connector check.
		{"owner sales passes ownership", owner, "sales", ReasonConnectorOffline},
		// Admin override preserved → passes ownership regardless of assignment.
		{"admin override", 99, "admin", ReasonConnectorOffline},
		// Legacy unauthenticated (userID=0) bypasses the ownership gate (preserved behavior).
		{"legacy unauthenticated bypass", 0, "", ReasonConnectorOffline},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, tc.userID, tc.role, accID); r != tc.want {
				t.Fatalf("user=%d role=%q → want %q, got %q", tc.userID, tc.role, tc.want, r)
			}
		})
	}
}

// seedAccountWithConnector seeds an owned account (optionally with a live fb_user_id)
// plus an online, logged-in extension connector assigned to it, with connectorFB as the
// connector's live Facebook identity. The connector shape mirrors the proven ready-seed
// (active + recent last_seen + supported version) so only connectorFB vs accountFB varies
// the PickReadyConnector outcome. Returns the account id.
func seedAccountWithConnector(t *testing.T, db *store.Store, orgID int64, name, accountFB, connectorFB string) int64 {
	t.Helper()
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: name, Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	if accountFB != "" {
		if _, err := db.DB().Exec(`UPDATE accounts SET fb_user_id = ? WHERE id = ?`, accountFB, accID); err != nil {
			t.Fatalf("set account fb: %v", err)
		}
	}
	if _, err := db.DB().Exec(
		`INSERT INTO agent_tokens (org_id, name, created_by, token_hash, kind, transport,
			assigned_account_id, fb_user_id, stream_status, version, active, last_seen, created_at)
		 VALUES (?, 'ext', 0, ?, 'extension_connector', 'chrome_extension', ?, ?,
			'facebook_logged_in', '9.9.9', 1, datetime('now'), datetime('now'))`,
		orgID, fmt.Sprintf("tok-%d", accID), accID, connectorFB,
	); err != nil {
		t.Fatalf("seed connector: %v", err)
	}
	return accID
}

// TestEvaluateCrawlAccountReadiness_ConnectorReasons pins how the connector-eligibility
// outcome (connectors.PickReadyConnector) maps onto readiness reason codes — the happy
// path AND the live-identity blocks — none of which the reject test covers. userID=0
// (legacy) bypasses the ownership gate so the CONNECTOR branch is what's exercised. The
// persisted actor-block path stays untouched (no verdict recorded), so the connector-side
// mismatch is the one under test.
func TestEvaluateCrawlAccountReadiness_ConnectorReasons(t *testing.T) {
	ctx := context.Background()
	dst := storetest.CopyTemplate(t, bootstrapReadinessStore, "crawl_readiness_connector")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(5)

	cases := []struct {
		name        string
		accountFB   string
		connectorFB string
		want        string
	}{
		// Distinct account fb per case: accounts.(org_id, fb_user_id) is UNIQUE. The
		// connector's live identity vs the account's is what varies the outcome.
		// Online + logged-in connector whose live identity matches the account → READY.
		{"ready", "111", "111", ReadinessReady},
		// Connector online but has resolved no Facebook identity (no c_user) → identity unknown.
		{"identity unknown", "112", "", ReasonActorIdentityUnknown},
		// Connector logged into a DIFFERENT Facebook than the account → live mismatch (blocked).
		{"identity mismatch", "113", "222", ReasonActorMismatchBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accID := seedAccountWithConnector(t, db, orgID, "acc-"+tc.name, tc.accountFB, tc.connectorFB)
			if r, _ := EvaluateCrawlAccountReadiness(ctx, db, orgID, 0, "admin", accID); r != tc.want {
				t.Fatalf("%s: want %q, got %q", tc.name, tc.want, r)
			}
		})
	}
}
