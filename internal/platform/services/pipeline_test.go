package services_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
	services "github.com/thg/scraper/internal/platform/services"
	"github.com/thg/scraper/internal/platform/services/adapters"
	"github.com/thg/scraper/internal/platform/services/contracts"
	"github.com/thg/scraper/internal/platform/services/resolver"
	"github.com/thg/scraper/internal/store"
)

// TestPipeline_StorageToContract snapshot-tests the full
// storage row -> adapter -> resolver -> contract path. If a storage change
// alters the contract semantics, this fails loudly.
func TestPipeline_StorageToContract(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "pipeline.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	orgID, err := db.CreateOrganization(&models.Organization{
		Name:        "Acme Co",
		PlanTier:    models.PlanFree,
		MaxAccounts: models.PlanFree.MaxAccounts(),
		Active:      true,
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	userID, err := db.CreateUser(&models.User{
		OrgID: orgID,
		Email: "admin@acme.test",
		Name:  "Acme Admin",
		Role:  models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	uc, err := adapters.LoadUserContext(db, userID)
	if err != nil {
		t.Fatalf("LoadUserContext: %v", err)
	}
	if !uc.Authenticated || uc.Org == nil || uc.Org.ID != orgID {
		t.Fatalf("unexpected UserContext: %+v", uc)
	}
	if uc.ResolvedAt == 0 {
		t.Fatalf("adapter must stamp ResolvedAt so resolvers stay pure")
	}

	fb := resolver.NewFacebookResolver()
	status := fb.ResolveStatus(uc)
	ws := fb.ResolveWorkspace(uc)
	access := fb.ResolveAccess(uc)
	caps := fb.ResolveCapabilities(uc)

	// Contract snapshot — these are the load-bearing semantics.
	if status != contracts.StatusAvailable {
		t.Errorf("status = %q, want available", status)
	}
	if ws.State != contracts.WorkspaceReady {
		t.Errorf("workspaceState = %q, want ready", ws.State)
	}
	wantWID := contracts.WorkspaceIDOf(orgID)
	if ws.WorkspaceID != wantWID {
		t.Errorf("workspaceId = %q, want %q", ws.WorkspaceID, wantWID)
	}
	if !strings.HasPrefix(ws.WorkspaceID, "ws_") {
		t.Errorf("workspaceId %q must carry the ws_ prefix", ws.WorkspaceID)
	}
	if access.Access != contracts.AccessGranted {
		t.Errorf("access = %q, want granted", access.Access)
	}
	if !caps.BrowserAutomation || !caps.AIAgents || caps.MultiWorkspace {
		t.Errorf("capabilities mismatch: %+v", caps)
	}
	if ws.Trace == nil || ws.Trace.Confidence != "authoritative" {
		t.Errorf("workspace trace must be authoritative, got %+v", ws.Trace)
	}
	// Resolver purity: it must use uc.ResolvedAt, not call time.Now itself.
	if ws.Trace.AuthoritativeAt != uc.ResolvedAt {
		t.Errorf("trace.AuthoritativeAt %d must equal uc.ResolvedAt %d (resolver must not call time.Now)",
			ws.Trace.AuthoritativeAt, uc.ResolvedAt)
	}
}

// TestPipeline_NoWorkspace covers a user with no organization (org_id = 0).
func TestPipeline_NoWorkspace(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "noworkspace.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	userID, err := db.CreateUser(&models.User{
		OrgID: 0,
		Email: "fresh@acme.test",
		Name:  "Fresh User",
		Role:  models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	uc, err := adapters.LoadUserContext(db, userID)
	if err != nil {
		t.Fatalf("LoadUserContext: %v", err)
	}
	if uc.Org != nil {
		t.Fatalf("expected nil Org for org_id=0 user, got %+v", uc.Org)
	}

	fb := resolver.NewFacebookResolver()
	ws := fb.ResolveWorkspace(uc)
	if ws.State != contracts.WorkspaceNone {
		t.Errorf("workspaceState = %q, want none", ws.State)
	}
	if ws.WorkspaceID != "" {
		t.Errorf("workspaceId = %q, want empty for no-workspace user", ws.WorkspaceID)
	}
}

// TestRegistry_Deterministic verifies the registry lists services in a stable
// order regardless of registration sequence.
func TestRegistry_Deterministic(t *testing.T) {
	r := services.DefaultRegistry()
	all := r.All()
	if len(all) == 0 {
		t.Fatal("DefaultRegistry must register at least one service")
	}
	// Slug must be present and the FB service first (displayOrder 10).
	if all[0].Descriptor().Slug != "facebook" {
		t.Errorf("first service = %q, want facebook", all[0].Descriptor().Slug)
	}
}
