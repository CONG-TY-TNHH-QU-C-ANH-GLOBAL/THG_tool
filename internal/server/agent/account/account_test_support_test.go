package account

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/testsupport"
)

// seedUnassignedAccountReadiness seeds one ACTIVE, UNASSIGNED Facebook account in
// org 5, builds the admin-scoped readiness matrix (an admin sees unassigned org
// accounts), and returns that account's row. Shared by the readiness/executable
// integration tests, which assert different facets of the same admin-visible-but-
// uncontrollable, no-connector row. The account name is irrelevant to those
// assertions, so a single fixture name is used.
func seedUnassignedAccountReadiness(t *testing.T, storeName string) *models.AccountReadiness {
	t.Helper()
	db := testsupport.NewTestStore(t, storeName)
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: 5, Platform: models.PlatformFacebook, Name: "unassigned", Status: models.AccountActive,
	}) // AssignedUserID = 0 (unassigned)
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	matrix, err := BuildAccountReadinessMatrix(db, 5, 1, "admin")
	if err != nil {
		t.Fatalf("BuildAccountReadinessMatrix: %v", err)
	}
	row := findReadinessRow(matrix, accID)
	if row == nil {
		t.Fatalf("account %d missing from admin matrix", accID)
	}
	return row
}

// findReadinessRow returns the readiness row for accID, or nil when absent.
func findReadinessRow(matrix []models.AccountReadiness, accID int64) *models.AccountReadiness {
	for i := range matrix {
		if matrix[i].AccountID == accID {
			return &matrix[i]
		}
	}
	return nil
}
