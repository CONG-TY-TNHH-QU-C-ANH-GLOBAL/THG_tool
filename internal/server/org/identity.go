package org

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/identities"
)

// ConnectorIdentitySnapshot captures everything the connector knows about the
// browser tab it is currently driving.
type ConnectorIdentitySnapshot struct {
	AccountID     int64
	OrgID         int64
	StreamStatus  string
	CurrentURL    string
	FBUserID      string
	FBDisplayName string
	FBUsername    string
	FBProfileURL  string
	LoginEmail    string
	ChromeError   string
}

// ResolvedConnectorIdentity reports what ResolveConnectorIdentity did so the
// HTTP handler can emit audit logs / typed errors (the helper itself stays free
// of HTTP + audit concerns — single responsibility).
type ResolvedConnectorIdentity struct {
	AccountID       int64 // the account the connector is now bound to
	Created         bool  // a brand-new account was created for this FB identity
	Rebound         bool  // the connector token's assigned_account_id changed
	PreviousAccount int64 // assigned account before the rebind
	Conflict        bool  // FB account owned by ANOTHER member — no mutation performed
	ConflictOwnerID int64
}

// ResolveConnectorIdentity is the Organic Sales Network connector→account
// resolver shared by the chrome-status heartbeat and the screenshot stream.
//
// When the snapshot shows a logged-in Facebook identity it maps that identity to
// its OWNING account (create-on-first-sight owned by createdBy), rebinds the
// connector token to that account, and persists the identity — the
// extension-reported snap.AccountID is only a hint. Ownership is never stolen: a
// FB account owned by a different member returns Conflict=true with no mutation.
// For a not-logged-in snapshot it falls back to the pre-assigned slot.
func ResolveConnectorIdentity(db *store.Store, ctx context.Context, agentID, createdBy, currentAssignedAccountID int64, snap ConnectorIdentitySnapshot) (ResolvedConnectorIdentity, error) {
	res := ResolvedConnectorIdentity{AccountID: snap.AccountID}
	if db == nil || snap.OrgID <= 0 {
		return res, nil
	}
	loggedIn := strings.EqualFold(strings.TrimSpace(snap.StreamStatus), browsergateway.StreamFacebookLoggedIn) &&
		strings.TrimSpace(snap.FBUserID) != ""
	if loggedIn {
		acc, created, err := db.Identities().ResolveOrCreateAccountForFacebookIdentity(
			snap.OrgID, createdBy, snap.FBUserID,
			identities.FacebookIdentityMeta{DisplayName: snap.FBDisplayName, Username: snap.FBUsername, ProfileURL: snap.FBProfileURL},
			snap.LoginEmail,
		)
		if err != nil {
			return res, err
		}
		if acc.AssignedUserID != 0 && createdBy != 0 && acc.AssignedUserID != createdBy {
			// No-steal: belongs to another member. Mutate nothing.
			return ResolvedConnectorIdentity{AccountID: acc.ID, Conflict: true, ConflictOwnerID: acc.AssignedUserID}, nil
		}
		res.AccountID = acc.ID
		res.Created = created
		if agentID > 0 && acc.ID != currentAssignedAccountID {
			if err := db.Connectors().AssignAgentAccount(agentID, snap.OrgID, acc.ID); err == nil {
				res.Rebound = true
				res.PreviousAccount = currentAssignedAccountID
			}
		}
		snap.AccountID = acc.ID
	} else if snap.AccountID > 0 {
		// Not logged in: only act on a slot that genuinely belongs to this org.
		if acc, err := db.Identities().GetAccountForOrg(snap.AccountID, snap.OrgID); err != nil || acc == nil {
			return res, nil
		}
	}
	if snap.AccountID > 0 {
		if err := ApplyConnectorIdentity(db, ctx, snap); err != nil {
			return res, err
		}
	}
	return res, nil
}

// ApplyConnectorIdentity persists the session row and, when the snapshot
// indicates a logged-in Facebook profile, the canonical account identity.
func ApplyConnectorIdentity(db *store.Store, ctx context.Context, snap ConnectorIdentitySnapshot) error {
	if db == nil || snap.AccountID <= 0 || snap.OrgID <= 0 {
		return nil
	}
	stream := strings.TrimSpace(snap.StreamStatus)
	if stream == "" {
		stream = browsergateway.StreamConnectorOnline
	}
	sessionStatus := store.LocalSessionStatusFromStream(stream)

	if appStore, err := store.NewAppStore(db); err == nil {
		_ = appStore.RecordLocalSession(ctx, snap.AccountID, snap.OrgID, sessionStatus, strings.TrimSpace(snap.ChromeError))
	}

	loggedIn := strings.EqualFold(stream, browsergateway.StreamFacebookLoggedIn) && strings.TrimSpace(snap.FBUserID) != ""
	if loggedIn {
		meta := identities.FacebookIdentityMeta{
			DisplayName: snap.FBDisplayName,
			Username:    snap.FBUsername,
			ProfileURL:  snap.FBProfileURL,
		}
		if err := db.Identities().SetAccountFacebookIdentity(snap.AccountID, snap.FBUserID, normalizeFacebookLoginEmail(snap.LoginEmail), meta); err != nil {
			return err
		}
		_ = db.Identities().UpdateAccountStatus(snap.AccountID, models.AccountActive)
		return nil
	}
	if store.LocalFacebookNotReady(stream) {
		_ = db.Identities().SetBrowserLoggedInState(snap.AccountID, false)
	}
	return nil
}

func normalizeFacebookLoginEmail(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > 320 {
		return ""
	}
	if strings.ContainsAny(value, " \t\r\n") || !strings.Contains(value, "@") {
		return ""
	}
	return value
}
