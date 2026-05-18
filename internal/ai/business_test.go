package ai

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/store"
)

func newBusinessTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// SaveForOrg writes under org:{id}:{key} so a write in org A must not be
// visible to a read in org B. This is the load-bearing cross-org guard
// for the new business-profile endpoint — if it ever regresses, one
// workspace will start serving another workspace's positioning.
func TestBusinessProfile_SaveForOrg_CrossOrgIsolation(t *testing.T) {
	db := newBusinessTestStore(t)

	orgA := &BusinessProfile{
		Name:        "Workspace A",
		Industry:    "fulfillment",
		Description: "POD fulfillment from VN/CN to global buyers",
		Services:    "sourcing, fulfillment, shipping",
		Targets:     "POD/dropship sellers looking for fulfillment",
	}
	orgB := &BusinessProfile{
		Name:        "Workspace B",
		Industry:    "recruitment",
		Description: "Tech hiring agency",
		Services:    "executive search, contract staffing",
		Targets:     "engineering teams hiring senior developers",
	}

	if err := orgA.SaveForOrg(db, 1); err != nil {
		t.Fatalf("SaveForOrg(1): %v", err)
	}
	if err := orgB.SaveForOrg(db, 2); err != nil {
		t.Fatalf("SaveForOrg(2): %v", err)
	}

	loadedA := LoadProfileForOrg(db, 1)
	if loadedA.Name != orgA.Name {
		t.Errorf("org 1 name: got %q want %q", loadedA.Name, orgA.Name)
	}
	if loadedA.Industry != orgA.Industry {
		t.Errorf("org 1 industry: got %q want %q", loadedA.Industry, orgA.Industry)
	}
	if loadedA.Description != orgA.Description {
		t.Errorf("org 1 description: got %q want %q", loadedA.Description, orgA.Description)
	}

	loadedB := LoadProfileForOrg(db, 2)
	if loadedB.Name != orgB.Name {
		t.Errorf("org 2 name: got %q want %q", loadedB.Name, orgB.Name)
	}
	if loadedB.Industry != orgB.Industry {
		t.Errorf("org 2 industry: got %q want %q", loadedB.Industry, orgB.Industry)
	}
	if loadedB.Description != orgB.Description {
		t.Errorf("org 2 description: got %q want %q", loadedB.Description, orgB.Description)
	}

	// Cross-org guard: reading org 1 must NOT see org 2's data, even
	// though both writes happened in the same store. A regression here
	// would mean either SaveForOrg stopped prefixing or LoadProfileForOrg
	// started reading from the wrong namespace.
	if loadedA.Industry == orgB.Industry {
		t.Errorf("org 1 leak: industry matches org 2 (%q)", loadedA.Industry)
	}
	if loadedB.Description == orgA.Description {
		t.Errorf("org 2 leak: description matches org 1 (%q)", loadedB.Description)
	}
}

// SaveForOrg with partial-update semantics: an unset field on the
// incoming struct must not blank a previously-saved value. This is the
// PUT-as-PATCH behavior the handler depends on, and matches updateOrg.
func TestBusinessProfile_SaveForOrg_PartialUpdate(t *testing.T) {
	db := newBusinessTestStore(t)

	first := &BusinessProfile{
		Name:        "Acme",
		Industry:    "fulfillment",
		Description: "initial description",
		USP:         "fast turnaround",
	}
	if err := first.SaveForOrg(db, 7); err != nil {
		t.Fatalf("SaveForOrg first: %v", err)
	}

	// Second save only sets Industry; USP and Description should remain.
	second := &BusinessProfile{
		Industry: "fulfillment_premium",
	}
	if err := second.SaveForOrg(db, 7); err != nil {
		t.Fatalf("SaveForOrg second: %v", err)
	}

	loaded := LoadProfileForOrg(db, 7)
	if loaded.Industry != "fulfillment_premium" {
		t.Errorf("industry should be updated; got %q", loaded.Industry)
	}
	if loaded.USP != "fast turnaround" {
		t.Errorf("USP should be preserved across partial update; got %q", loaded.USP)
	}
	if loaded.Description != "initial description" {
		t.Errorf("description should be preserved; got %q", loaded.Description)
	}
}

// SaveForOrg rejects bad inputs rather than corrupting global state.
func TestBusinessProfile_SaveForOrg_RejectsBadInputs(t *testing.T) {
	db := newBusinessTestStore(t)
	p := &BusinessProfile{Name: "test"}

	if err := p.SaveForOrg(nil, 1); err == nil {
		t.Error("SaveForOrg(nil db) should error")
	}
	if err := p.SaveForOrg(db, 0); err == nil {
		t.Error("SaveForOrg(orgID=0) should error — would write to global namespace")
	}
	if err := p.SaveForOrg(db, -1); err == nil {
		t.Error("SaveForOrg(orgID<0) should error")
	}
}
