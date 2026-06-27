package connectors_test

import (
	"errors"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
)

// Per-subtest assertion helpers for TestPairingOwnershipBoundary. Each is a small,
// S3776-clean step run in sequence against the SAME store (later steps depend on
// earlier claims). Split into their own file to keep each file <= 200 lines.

func assertOneUserMultiProfile(t *testing.T, db *store.Store) {
	t.Helper()
	if _, err := claimProfile(t, db, poOrgX, poUserA, "profile-a1"); err != nil {
		t.Fatalf("profile-a1: %v", err)
	}
	if _, err := claimProfile(t, db, poOrgX, poUserA, "profile-a2"); err != nil {
		t.Fatalf("profile-a2: %v", err)
	}
}

func assertSecondUserDifferentProfile(t *testing.T, db *store.Store) {
	t.Helper()
	if _, err := claimProfile(t, db, poOrgX, poUserB, "profile-b1"); err != nil {
		t.Fatalf("profile-b1: %v", err)
	}
}

func assertNoCrossUserRepair(t *testing.T, db *store.Store) {
	t.Helper()
	_, err := claimProfile(t, db, poOrgX, poUserB, "profile-a1")
	if !errors.Is(err, connectors.ErrDevicePairedToAnotherUser) {
		t.Fatalf("want ErrDevicePairedToAnotherUser, got %v", err)
	}
	// Original owner's binding stays intact.
	if !hasActiveProfile(t, db, poOrgX, poUserA, "profile-a1") {
		t.Fatalf("user A's profile-a1 binding must survive the blocked re-pair")
	}
}

func assertNoCrossWorkspaceRepair(t *testing.T, db *store.Store) {
	t.Helper()
	_, err := claimProfile(t, db, poOrgY, poUserA, "profile-a1")
	if !errors.Is(err, connectors.ErrDevicePairedToAnotherWorkspace) {
		t.Fatalf("want ErrDevicePairedToAnotherWorkspace, got %v", err)
	}
}

func assertSameOwnerRepairReplaces(t *testing.T, db *store.Store) {
	t.Helper()
	first, err := claimProfile(t, db, poOrgX, poUserA, "profile-repair")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := claimProfile(t, db, poOrgX, poUserA, "profile-repair")
	if err != nil {
		t.Fatalf("re-pair by same owner must succeed: %v", err)
	}
	if second.Token.ID == first.Token.ID {
		t.Fatalf("re-pair must mint a new connector token")
	}
	if n := countActiveProfile(t, db, poOrgX, "profile-repair"); n != 1 {
		t.Fatalf("want exactly 1 active connector for the profile, got %d", n)
	}
}

func assertRepairAfterForget(t *testing.T, db *store.Store) {
	t.Helper()
	first, err := claimProfile(t, db, poOrgX, poUserA, "profile-forget")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := db.Connectors().RevokeAgentToken(first.Token.ID, poOrgX); err != nil {
		t.Fatalf("RevokeAgentToken: %v", err)
	}
	if _, err := claimProfile(t, db, poOrgX, poUserB, "profile-forget"); err != nil {
		t.Fatalf("claim after forget device must succeed: %v", err)
	}
}

func assertProfileIDRequired(t *testing.T, db *store.Store) {
	t.Helper()
	// A NEW claim with no browser_profile_id must be refused (force-update),
	// never silently paired — else the no-steal guard is bypassable.
	_, err := claimProfile(t, db, poOrgX, poUserA, "")
	if !errors.Is(err, connectors.ErrBrowserProfileRequired) {
		t.Fatalf("want ErrBrowserProfileRequired, got %v", err)
	}
	if _, err := claimProfile(t, db, poOrgX, poUserB, "  "); !errors.Is(err, connectors.ErrBrowserProfileRequired) {
		t.Fatalf("whitespace-only id must be rejected too, got %v", err)
	}
	if n := countActiveProfile(t, db, poOrgX, ""); n != 0 {
		t.Fatalf("blocked claims must not mint a connector, got %d empty-profile tokens", n)
	}
}

func assertCodeLifecyclePrecedence(t *testing.T, db *store.Store) {
	t.Helper()
	// An invalid/used/expired code reports its own reason before the profile gate.
	pair, err := db.Connectors().CreateConnectorPairingCode("dev", poUserA, poOrgX, 0, time.Minute)
	if err != nil {
		t.Fatalf("CreateConnectorPairingCode: %v", err)
	}
	valid := connectors.AgentPresence{BrowserProfileID: "profile-lifecycle"}
	if _, err := db.Connectors().ClaimConnectorPairingCode(pair.Code, valid); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := db.Connectors().ClaimConnectorPairingCode(pair.Code, connectors.AgentPresence{}); !errors.Is(err, connectors.ErrPairingCodeConsumed) {
		t.Fatalf("want ErrPairingCodeConsumed, got %v", err)
	}
	if _, err := db.Connectors().ClaimConnectorPairingCode("ZZZZ-9999", connectors.AgentPresence{}); !errors.Is(err, connectors.ErrPairingCodeInvalid) {
		t.Fatalf("want ErrPairingCodeInvalid, got %v", err)
	}
	expired, err := db.Connectors().CreateConnectorPairingCode("dev", poUserA, poOrgX, 0, time.Millisecond)
	if err != nil {
		t.Fatalf("CreateConnectorPairingCode (expired): %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := db.Connectors().ClaimConnectorPairingCode(expired.Code, connectors.AgentPresence{}); !errors.Is(err, connectors.ErrPairingCodeExpired) {
		t.Fatalf("want ErrPairingCodeExpired, got %v", err)
	}
}

func assertClaimExposesSession(t *testing.T, db *store.Store) {
	t.Helper()
	pair, err := db.Connectors().CreateConnectorPairingCode("dev", poUserA, poOrgX, 0, time.Minute)
	if err != nil {
		t.Fatalf("CreateConnectorPairingCode: %v", err)
	}
	claimed, err := db.Connectors().ClaimConnectorPairingCode(pair.Code, connectors.AgentPresence{BrowserProfileID: "profile-session"})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.PairingSessionID != pair.ID {
		t.Fatalf("want pairing_session_id %d, got %d", pair.ID, claimed.PairingSessionID)
	}
	sess, err := db.Connectors().GetConnectorPairingSession(pair.ID, poOrgX)
	if err != nil || sess == nil {
		t.Fatalf("GetConnectorPairingSession: sess=%v err=%v", sess, err)
	}
	if sess.DeviceTokenID != claimed.Token.ID || !sess.Used || sess.CreatedBy != poUserA {
		t.Fatalf("session binding mismatch: %+v vs token %d", sess, claimed.Token.ID)
	}
	if other, err := db.Connectors().GetConnectorPairingSession(pair.ID, poOrgY); err != nil || other != nil {
		t.Fatalf("session must be invisible outside its workspace, got %+v err=%v", other, err)
	}
}
