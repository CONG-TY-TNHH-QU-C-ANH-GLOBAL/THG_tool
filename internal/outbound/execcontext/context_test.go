package execcontext

import (
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// TestResolveCallerAccountID_DeterministicContext pins the Organic Sales Network
// deterministic ExecutionContext: explicit -> Default Account -> exactly-one ->
// error execution_context_required. NO heuristic / first-logged-in guessing.
func TestResolveCallerAccountID_DeterministicContext(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "exec_ctx_test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	const org, member int64 = 1, 7
	acc1, _ := db.Identities().AddAccount(&models.Account{
		OrgID: org, Platform: models.PlatformFacebook, Name: "FB1", AssignedUserID: member, Status: models.AccountActive,
	})

	// Exactly one owned account → deterministic, resolves to it (not a guess).
	if got, err := ResolveCallerAccountID(db, org, member, "sales", 0, true); err != nil || got != acc1 {
		t.Fatalf("exactly-one: got %d err %v, want %d", got, err, acc1)
	}

	// A second owned account → ambiguous with no default → must error.
	acc2, _ := db.Identities().AddAccount(&models.Account{
		OrgID: org, Platform: models.PlatformFacebook, Name: "FB2", AssignedUserID: member, Status: models.AccountActive,
	})
	if _, err := ResolveCallerAccountID(db, org, member, "sales", 0, true); err == nil {
		t.Fatal("ambiguous (2 accounts, no default) must error execution_context_required")
	}

	// Setting a Default Account makes it deterministic again.
	if err := db.SetUserDefaultAccount(org, member, acc2, "sales"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if got, err := ResolveCallerAccountID(db, org, member, "sales", 0, true); err != nil || got != acc2 {
		t.Fatalf("default: got %d err %v, want %d", got, err, acc2)
	}

	// Cannot set a default to an account you don't own.
	other, _ := db.Identities().AddAccount(&models.Account{
		OrgID: org, Platform: models.PlatformFacebook, Name: "Other", AssignedUserID: 99, Status: models.AccountActive,
	})
	if err := db.SetUserDefaultAccount(org, member, other, "sales"); err == nil {
		t.Fatal("setting default to a non-owned account must error")
	}
}

// TestResolveUserActionContext checks the campaign-ready ActionContext: a
// member-initiated resolution yields Source=manual with the immutable
// InitiatorUserID and the deterministically resolved AccountID.
func TestResolveUserActionContext(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "actx_test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer db.Close()

	const org, member int64 = 1, 7
	acc, _ := db.Identities().AddAccount(&models.Account{
		OrgID: org, Platform: models.PlatformFacebook, Name: "FB", AssignedUserID: member, Status: models.AccountActive,
	})

	actx, err := ResolveUserActionContext(db, org, member, "sales", 0, true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if actx.Source != models.ActionSourceManual {
		t.Fatalf("source = %q, want manual", actx.Source)
	}
	if actx.InitiatorUserID != member || actx.AccountID != acc || actx.OrgID != org {
		t.Fatalf("context wrong: %+v (want org=%d user=%d acc=%d)", actx, org, member, acc)
	}
	if actx.ConnectorID != 0 || actx.CampaignID != 0 {
		t.Fatalf("connectorless/no-campaign defaults expected, got %+v", actx)
	}
}
