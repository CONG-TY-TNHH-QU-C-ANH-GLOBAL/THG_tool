package server

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/browsergateway"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/store"
)

// connectorIdentitySnapshot captures everything the connector knows about the
// browser tab it is currently driving. The same shape is reported by
// /api/agent/heartbeat (presence ping), /api/agent/chrome-status (handshake
// before workspace exists) and /api/agent/screenshot (frame stream).
//
// Handlers convert their request body into this struct and hand it to
// applyConnectorIdentity which is the single place that updates browser_sessions,
// the account's FB identity, and the account active flag.
type connectorIdentitySnapshot struct {
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

// applyConnectorIdentity persists the session row and, when the snapshot
// indicates a logged-in Facebook profile, the canonical account identity.
//
// Returns:
//   - false, nil when the snapshot was applied without a hard conflict.
//   - false, identityError when SetAccountFacebookIdentity rejected the
//     update (caller should map this to HTTP 409 — Facebook profile mismatch
//     against the account slot).
//
// applyConnectorIdentity does NOT enforce the FB profile mismatch guard —
// callers must run rejectIfFacebookProfileMismatch first when they want to
// short-circuit before mutating any state. This separation lets the heartbeat
// path (which only updates presence) skip the strict 409 behaviour while the
// screenshot/chrome-status paths still enforce it.
func (s *Server) applyConnectorIdentity(ctx context.Context, snap connectorIdentitySnapshot) error {
	if snap.AccountID <= 0 || snap.OrgID <= 0 {
		return nil
	}
	stream := strings.TrimSpace(snap.StreamStatus)
	if stream == "" {
		stream = browsergateway.StreamConnectorOnline
	}
	sessionStatus := store.LocalSessionStatusFromStream(stream)

	if appStore, err := store.NewAppStore(s.db); err == nil {
		_ = appStore.RecordLocalSession(ctx, snap.AccountID, snap.OrgID, sessionStatus, strings.TrimSpace(snap.ChromeError))
	}

	loggedIn := strings.EqualFold(stream, browsergateway.StreamFacebookLoggedIn) && strings.TrimSpace(snap.FBUserID) != ""
	if loggedIn {
		meta := store.FacebookIdentityMeta{
			DisplayName: snap.FBDisplayName,
			Username:    snap.FBUsername,
			ProfileURL:  snap.FBProfileURL,
		}
		if err := s.db.SetAccountFacebookIdentity(snap.AccountID, snap.FBUserID, normalizeFacebookLoginEmail(snap.LoginEmail), meta); err != nil {
			return err
		}
		_ = s.db.UpdateAccountStatus(snap.AccountID, models.AccountActive)
		return nil
	}
	if store.LocalFacebookNotReady(stream) {
		_ = s.db.SetBrowserLoggedInState(snap.AccountID, false)
	}
	return nil
}
