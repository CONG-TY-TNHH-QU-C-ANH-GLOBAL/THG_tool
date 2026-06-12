package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapPairing(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func claimProfile(t *testing.T, db *store.Store, orgID, userID int64, profileID string) (*connectors.ClaimedPairing, error) {
	t.Helper()
	pair, err := db.Connectors().CreateConnectorPairingCode("dev", userID, orgID, 0, time.Minute)
	if err != nil {
		t.Fatalf("CreateConnectorPairingCode: %v", err)
	}
	return db.Connectors().ClaimConnectorPairingCode(pair.Code, connectors.AgentPresence{BrowserProfileID: profileID})
}

// TestPairingOwnershipBoundary covers the Chrome-profile (extension instance)
// ownership boundary: the Chrome profile binds — not the physical device — so
// many profiles and many users may share one laptop, but one profile must not
// silently re-bind across users or workspaces.
func TestPairingOwnershipBoundary(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapPairing, "pairing_ownership")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgX, orgY, userA, userB = int64(1), int64(2), int64(100), int64(200)

	t.Run("one user pairs multiple chrome profiles in one workspace", func(t *testing.T) {
		if _, err := claimProfile(t, db, orgX, userA, "profile-a1"); err != nil {
			t.Fatalf("profile-a1: %v", err)
		}
		if _, err := claimProfile(t, db, orgX, userA, "profile-a2"); err != nil {
			t.Fatalf("profile-a2: %v", err)
		}
	})

	t.Run("second user on same laptop pairs a different chrome profile", func(t *testing.T) {
		if _, err := claimProfile(t, db, orgX, userB, "profile-b1"); err != nil {
			t.Fatalf("profile-b1: %v", err)
		}
	})

	t.Run("same chrome profile cannot silently re-pair to another user", func(t *testing.T) {
		_, err := claimProfile(t, db, orgX, userB, "profile-a1")
		if !errors.Is(err, connectors.ErrDevicePairedToAnotherUser) {
			t.Fatalf("want ErrDevicePairedToAnotherUser, got %v", err)
		}
		// Original owner's binding stays intact.
		if !hasActiveProfile(t, db, orgX, userA, "profile-a1") {
			t.Fatalf("user A's profile-a1 binding must survive the blocked re-pair")
		}
	})

	t.Run("same chrome profile cannot silently re-pair to another workspace", func(t *testing.T) {
		_, err := claimProfile(t, db, orgY, userA, "profile-a1")
		if !errors.Is(err, connectors.ErrDevicePairedToAnotherWorkspace) {
			t.Fatalf("want ErrDevicePairedToAnotherWorkspace, got %v", err)
		}
	})

	t.Run("same owner re-pair replaces the old binding", func(t *testing.T) {
		first, err := claimProfile(t, db, orgX, userA, "profile-repair")
		if err != nil {
			t.Fatalf("first claim: %v", err)
		}
		second, err := claimProfile(t, db, orgX, userA, "profile-repair")
		if err != nil {
			t.Fatalf("re-pair by same owner must succeed: %v", err)
		}
		if second.Token.ID == first.Token.ID {
			t.Fatalf("re-pair must mint a new connector token")
		}
		if n := countActiveProfile(t, db, orgX, "profile-repair"); n != 1 {
			t.Fatalf("want exactly 1 active connector for the profile, got %d", n)
		}
	})

	t.Run("re-pair by another user allowed after forget device", func(t *testing.T) {
		first, err := claimProfile(t, db, orgX, userA, "profile-forget")
		if err != nil {
			t.Fatalf("first claim: %v", err)
		}
		if err := db.Connectors().RevokeAgentToken(first.Token.ID, orgX); err != nil {
			t.Fatalf("RevokeAgentToken: %v", err)
		}
		if _, err := claimProfile(t, db, orgX, userB, "profile-forget"); err != nil {
			t.Fatalf("claim after forget device must succeed: %v", err)
		}
	})

	t.Run("legacy extension without profile id skips the guard", func(t *testing.T) {
		if _, err := claimProfile(t, db, orgX, userA, ""); err != nil {
			t.Fatalf("legacy claim A: %v", err)
		}
		if _, err := claimProfile(t, db, orgX, userB, ""); err != nil {
			t.Fatalf("legacy claim B: %v", err)
		}
	})

	t.Run("typed code lifecycle errors", func(t *testing.T) {
		pair, err := db.Connectors().CreateConnectorPairingCode("dev", userA, orgX, 0, time.Minute)
		if err != nil {
			t.Fatalf("CreateConnectorPairingCode: %v", err)
		}
		if _, err := db.Connectors().ClaimConnectorPairingCode(pair.Code, connectors.AgentPresence{}); err != nil {
			t.Fatalf("first claim: %v", err)
		}
		if _, err := db.Connectors().ClaimConnectorPairingCode(pair.Code, connectors.AgentPresence{}); !errors.Is(err, connectors.ErrPairingCodeConsumed) {
			t.Fatalf("want ErrPairingCodeConsumed, got %v", err)
		}
		if _, err := db.Connectors().ClaimConnectorPairingCode("ZZZZ-9999", connectors.AgentPresence{}); !errors.Is(err, connectors.ErrPairingCodeInvalid) {
			t.Fatalf("want ErrPairingCodeInvalid, got %v", err)
		}
		expired, err := db.Connectors().CreateConnectorPairingCode("dev", userA, orgX, 0, time.Millisecond)
		if err != nil {
			t.Fatalf("CreateConnectorPairingCode (expired): %v", err)
		}
		time.Sleep(10 * time.Millisecond)
		if _, err := db.Connectors().ClaimConnectorPairingCode(expired.Code, connectors.AgentPresence{}); !errors.Is(err, connectors.ErrPairingCodeExpired) {
			t.Fatalf("want ErrPairingCodeExpired, got %v", err)
		}
	})

	t.Run("claimed pairing exposes the session id for scoped verification", func(t *testing.T) {
		pair, err := db.Connectors().CreateConnectorPairingCode("dev", userA, orgX, 0, time.Minute)
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
		sess, err := db.Connectors().GetConnectorPairingSession(pair.ID, orgX)
		if err != nil || sess == nil {
			t.Fatalf("GetConnectorPairingSession: sess=%v err=%v", sess, err)
		}
		if sess.DeviceTokenID != claimed.Token.ID || !sess.Used || sess.CreatedBy != userA {
			t.Fatalf("session binding mismatch: %+v vs token %d", sess, claimed.Token.ID)
		}
		if other, err := db.Connectors().GetConnectorPairingSession(pair.ID, orgY); err != nil || other != nil {
			t.Fatalf("session must be invisible outside its workspace, got %+v err=%v", other, err)
		}
	})
}

func hasActiveProfile(t *testing.T, db *store.Store, orgID, userID int64, profileID string) bool {
	t.Helper()
	for _, c := range listActiveConnectors(t, db, orgID) {
		if c.BrowserProfileID == profileID && c.CreatedBy == userID {
			return true
		}
	}
	return false
}

func countActiveProfile(t *testing.T, db *store.Store, orgID int64, profileID string) int {
	t.Helper()
	n := 0
	for _, c := range listActiveConnectors(t, db, orgID) {
		if c.BrowserProfileID == profileID {
			n++
		}
	}
	return n
}

func listActiveConnectors(t *testing.T, db *store.Store, orgID int64) []connectors.AgentToken {
	t.Helper()
	conns, err := db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		t.Fatalf("ListLocalConnectors: %v", err)
	}
	return conns
}
