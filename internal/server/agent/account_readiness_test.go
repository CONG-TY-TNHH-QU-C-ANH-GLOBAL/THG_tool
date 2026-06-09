package agent

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrapReadinessMatrixStore(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

// PR-D: a seeded account with NO online connector is not ready for any capability,
// and every capability reports the connector_offline reason.
func TestBuildAccountReadinessMatrix_NoConnector(t *testing.T) {
	dst := storetest.CopyTemplate(t, bootstrapReadinessMatrixStore, "readiness_matrix")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const orgID = int64(5)

	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "acc-a", Status: models.AccountActive,
	})
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}

	// admin sees unassigned org accounts (RBAC privacy rule).
	matrix, err := BuildAccountReadinessMatrix(db, orgID, 1, "admin")
	if err != nil {
		t.Fatalf("BuildAccountReadinessMatrix: %v", err)
	}
	var found *models.AccountReadiness
	for i := range matrix {
		if matrix[i].AccountID == accID {
			found = &matrix[i]
		}
	}
	if found == nil {
		t.Fatalf("account %d missing from matrix", accID)
	}
	if len(found.Capabilities) != 4 {
		t.Fatalf("want 4 capabilities, got %d", len(found.Capabilities))
	}
	for _, cap := range found.Capabilities {
		if cap.Can {
			t.Fatalf("capability %q should not be ready (no connector)", cap.Capability)
		}
		if len(cap.Reasons) == 0 || cap.Reasons[0] != "connector_offline" {
			t.Fatalf("capability %q reasons = %v, want connector_offline first", cap.Capability, cap.Reasons)
		}
	}
	if found.RequiredAction == "" {
		t.Fatalf("required_action should hint at the connector fix")
	}
}
