package org

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
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
		meta := store.FacebookIdentityMeta{
			DisplayName: snap.FBDisplayName,
			Username:    snap.FBUsername,
			ProfileURL:  snap.FBProfileURL,
		}
		if err := db.SetAccountFacebookIdentity(snap.AccountID, snap.FBUserID, normalizeFacebookLoginEmail(snap.LoginEmail), meta); err != nil {
			return err
		}
		_ = db.UpdateAccountStatus(snap.AccountID, models.AccountActive)
		return nil
	}
	if store.LocalFacebookNotReady(stream) {
		_ = db.SetBrowserLoggedInState(snap.AccountID, false)
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
