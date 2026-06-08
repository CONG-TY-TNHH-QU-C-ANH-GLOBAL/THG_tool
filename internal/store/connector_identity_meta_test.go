package store_test

import (
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/connectors"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapConnIdentity(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// TestConnectorIdentityMetadataPersist covers FB Reliability PR-B/B3: the
// heartbeat-supplied identity_confidence / identity_extraction_method /
// identity_last_verified_at round-trip through UpdateAgentPresence →
// ListLocalConnectors, and a later heartbeat that omits them PRESERVES the last
// known values (CASE WHEN != '').
func TestConnectorIdentityMetadataPersist(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapConnIdentity, "conn_identity")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(5)

	id, _, err := db.Connectors().CreateAgentToken("ext", 0, orgID)
	if err != nil {
		t.Fatalf("CreateAgentToken: %v", err)
	}
	if err := db.Connectors().UpdateAgentPresence(id, connectors.AgentPresence{
		Kind:                     "extension_connector",
		AssignedAccountID:        50,
		FBUserID:                 "111",
		StreamStatus:             "facebook_logged_in",
		IdentityConfidence:       "high",
		IdentityExtractionMethod: "cookie_c_user",
		IdentityLastVerifiedAt:   "2026-06-09T00:00:00Z",
		BrowserProfileID:         "profile-uuid-abc",
	}); err != nil {
		t.Fatalf("UpdateAgentPresence: %v", err)
	}

	conns, err := db.Connectors().ListLocalConnectors(orgID)
	if err != nil {
		t.Fatalf("ListLocalConnectors: %v", err)
	}
	if len(conns) != 1 {
		t.Fatalf("want 1 connector, got %d", len(conns))
	}
	c := conns[0]
	if c.IdentityConfidence != "high" || c.IdentityExtractionMethod != "cookie_c_user" || c.IdentityLastVerifiedAt != "2026-06-09T00:00:00Z" {
		t.Fatalf("identity metadata not persisted: confidence=%q method=%q verified=%q", c.IdentityConfidence, c.IdentityExtractionMethod, c.IdentityLastVerifiedAt)
	}
	if c.BrowserProfileID != "profile-uuid-abc" {
		t.Fatalf("browser_profile_id not persisted: %q", c.BrowserProfileID)
	}

	// A later heartbeat that omits the identity fields must PRESERVE the last
	// known values (old extension / transient gap must not wipe identity).
	if err := db.Connectors().UpdateAgentPresence(id, connectors.AgentPresence{
		Kind: "extension_connector", StreamStatus: "facebook_logged_in",
	}); err != nil {
		t.Fatalf("UpdateAgentPresence (omit identity): %v", err)
	}
	conns, _ = db.Connectors().ListLocalConnectors(orgID)
	if conns[0].IdentityConfidence != "high" || conns[0].IdentityExtractionMethod != "cookie_c_user" {
		t.Fatalf("identity metadata should be preserved when omitted, got confidence=%q method=%q", conns[0].IdentityConfidence, conns[0].IdentityExtractionMethod)
	}
}
