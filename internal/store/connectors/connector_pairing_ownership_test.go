package connectors_test

import (
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

// Workspace/user identities shared by the ownership-boundary subtests, which run
// against ONE store in sequence (later cases depend on earlier claims).
const (
	poOrgX  = int64(1)
	poOrgY  = int64(2)
	poUserA = int64(100)
	poUserB = int64(200)
)

// TestPairingOwnershipBoundary covers the Chrome-profile (extension instance)
// ownership boundary: the Chrome profile binds — not the physical device — so
// many profiles and many users may share one laptop, but one profile must not
// silently re-bind across users or workspaces. The subtests share one store and
// run in order; each body is a named helper (kept small so the whole stays
// S3776-clean) but the shared-db sequence is unchanged.
func TestPairingOwnershipBoundary(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapPairing, "pairing_ownership")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	t.Run("one user pairs multiple chrome profiles in one workspace", func(t *testing.T) { assertOneUserMultiProfile(t, db) })
	t.Run("second user on same laptop pairs a different chrome profile", func(t *testing.T) { assertSecondUserDifferentProfile(t, db) })
	t.Run("same chrome profile cannot silently re-pair to another user", func(t *testing.T) { assertNoCrossUserRepair(t, db) })
	t.Run("same chrome profile cannot silently re-pair to another workspace", func(t *testing.T) { assertNoCrossWorkspaceRepair(t, db) })
	t.Run("same owner re-pair replaces the old binding", func(t *testing.T) { assertSameOwnerRepairReplaces(t, db) })
	t.Run("re-pair by another user allowed after forget device", func(t *testing.T) { assertRepairAfterForget(t, db) })
	t.Run("new pairing without a stable profile id is blocked, not bypassed", func(t *testing.T) { assertProfileIDRequired(t, db) })
	t.Run("typed code lifecycle errors take precedence over profile requirement", func(t *testing.T) { assertCodeLifecyclePrecedence(t, db) })
	t.Run("claimed pairing exposes the session id for scoped verification", func(t *testing.T) { assertClaimExposesSession(t, db) })
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
