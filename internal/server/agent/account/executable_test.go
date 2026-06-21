package account

import (
	"testing"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/server/testsupport"
	"github.com/thg/scraper/internal/store/connectors"
)

// --- pure resolver tests (no DB): the 8 typed executability states ---

func execAcc(id, owner int64, fb string) models.Account {
	return models.Account{
		ID: id, OrgID: 5, Platform: models.PlatformFacebook, Name: "a",
		Status: models.AccountActive, AssignedUserID: owner, FBUserID: fb,
	}
}

func execConn(createdBy, assigned int64, fb, stream string, online bool) connectors.AgentToken {
	return connectors.AgentToken{
		CreatedBy: createdBy, AssignedAccountID: assigned, FBUserID: fb,
		StreamStatus: stream, Version: "9.9.9", Active: true, Online: online,
	}
}

func TestResolveAccountExecutable_States(t *testing.T) {
	const userA, userB int64 = 11, 22
	const accID int64 = 1
	pol := connectors.VersionPolicy{}
	live := func(conns []connectors.AgentToken) []connectors.AgentToken {
		return connectors.OwnedBy(conns, userA) // mirror the caller: scope to requester-owned
	}

	cases := []struct {
		name       string
		acc        models.Account
		conns      []connectors.AgentToken
		actorBlock bool
		active     bool
		wantCode   string
		wantExec   bool
	}{
		{"no_connector (none owned)", execAcc(accID, userA, "fbA"), nil, false, true, models.ExecReasonNoConnector, false},
		{"connector_stale (owned, offline)", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "fbA", "facebook_logged_in", false)}, false, true, models.ExecReasonConnectorStale, false},
		{"identity_mismatch", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "OTHER", "facebook_logged_in", true)}, false, true, models.ExecReasonIdentityMismatch, false},
		{"pairing_pending (online, not logged in)", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "", "waiting_pairing", true)}, false, true, models.ExecReasonPairingPending, false},
		{"session_blocked (checkpoint)", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "fbA", "facebook_checkpoint", true)}, false, true, models.ExecReasonSessionBlocked, false},
		{"account_blocked (suspended)", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "fbA", "facebook_logged_in", true)}, false, false, models.ExecReasonAccountBlocked, false},
		{"account_blocked (actor block)", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "fbA", "facebook_logged_in", true)}, true, true, models.ExecReasonAccountBlocked, false},
		{"not_controllable (member account)", execAcc(accID, userB, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "fbA", "facebook_logged_in", true)}, false, true, models.ExecReasonNotControllable, false},
		{"ready (own live matched connector)", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userA, accID, "fbA", "facebook_logged_in", true)}, false, true, models.ExecReasonReady, true},
		// Security: a fresh connector owned by ANOTHER user must NOT make the requester executable.
		{"org-wide other-user connector → no_connector", execAcc(accID, userA, "fbA"),
			[]connectors.AgentToken{execConn(userB, accID, "fbA", "facebook_logged_in", true)}, false, true, models.ExecReasonNoConnector, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := resolveAccountExecutable(tc.acc, live(tc.conns), pol, tc.actorBlock, tc.active, userA)
			if st.reasonCode != tc.wantCode {
				t.Errorf("reasonCode = %q, want %q", st.reasonCode, tc.wantCode)
			}
			if st.executable != tc.wantExec {
				t.Errorf("executable = %v, want %v", st.executable, tc.wantExec)
			}
			if tc.wantExec && (!st.controlAllowed || !st.connectorOnline || !st.liveIdentityMatched || !st.sessionUsable) {
				t.Errorf("ready must set all liveness booleans, got %+v", st)
			}
		})
	}
}

func TestConnectorsOwnedBy(t *testing.T) {
	conns := []connectors.AgentToken{
		execConn(11, 1, "fbA", "facebook_logged_in", true),
		execConn(22, 2, "fbB", "facebook_logged_in", true),
		{CreatedBy: 0, AssignedAccountID: 3}, // legacy untracked owner
	}
	if got := connectors.OwnedBy(conns, 11); len(got) != 1 || got[0].CreatedBy != 11 {
		t.Errorf("OwnedBy(11) must return only user 11's connector, got %+v", got)
	}
	if got := connectors.OwnedBy(conns, 0); got != nil {
		t.Errorf("OwnedBy(0) must own nothing (fail closed), got %+v", got)
	}
}

// --- integration: admin inventory visibility must NOT imply executable control ---

// An admin can VIEW an unassigned org account (CanViewAccountDevice), but with no assignment
// it is NOT controllable → executable=false, not_controllable. Visibility ≠ control.
func TestBuildAccountReadinessMatrix_AdminVisibilityNotControl(t *testing.T) {
	db := testsupport.NewTestStore(t, "exec_matrix")
	const orgID = int64(5)
	accID, err := db.Identities().AddAccount(&models.Account{
		OrgID: orgID, Platform: models.PlatformFacebook, Name: "unassigned", Status: models.AccountActive,
	}) // AssignedUserID = 0 (unassigned)
	if err != nil {
		t.Fatalf("AddAccount: %v", err)
	}
	matrix, err := BuildAccountReadinessMatrix(db, orgID, 1, "admin")
	if err != nil {
		t.Fatalf("matrix: %v", err)
	}
	var found *models.AccountReadiness
	for i := range matrix {
		if matrix[i].AccountID == accID {
			found = &matrix[i]
		}
	}
	if found == nil {
		t.Fatalf("admin should VIEW the unassigned account, but it is missing")
	}
	if found.Executable || found.ControlAllowed {
		t.Errorf("admin visibility must not imply control/executable, got %+v", found)
	}
	if found.ExecReasonCode != models.ExecReasonNotControllable {
		t.Errorf("unassigned-for-admin must be not_controllable, got %q", found.ExecReasonCode)
	}
}
