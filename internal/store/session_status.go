// Domain: sessions (see internal/store/DOMAINS.md)
package store

import (
	"context"

	"github.com/thg/scraper/internal/store/sessions"
)

// LocalSessionStatus is an alias of [sessions.LocalSessionStatus] for source
// compatibility. New code should import "internal/store/sessions" and use
// [sessions.LocalSessionStatus] directly.
type LocalSessionStatus = sessions.LocalSessionStatus

// Session status constants — re-exports for source compatibility.
//
// New code should use the sessions package constants directly.
const (
	SessionStarting     = sessions.SessionStarting
	SessionActive       = sessions.SessionActive
	SessionReady        = sessions.SessionReady
	SessionLoginReq     = sessions.SessionLoginReq
	SessionHumanReq     = sessions.SessionHumanReq
	SessionError        = sessions.SessionError
	SessionTerminated   = sessions.SessionTerminated
	SessionInitializing = sessions.SessionInitializing
	SessionDisplayReady = sessions.SessionDisplayReady
	SessionCheckpoint   = sessions.SessionCheckpoint
	SessionIdle         = sessions.SessionIdle
	SessionErrorState   = sessions.SessionErrorState
)

// LocalSessionStatusFromStream delegates to [sessions.LocalSessionStatusFromStream].
func LocalSessionStatusFromStream(stream string) LocalSessionStatus {
	return sessions.LocalSessionStatusFromStream(stream)
}

// LocalFacebookNotReady delegates to [sessions.LocalFacebookNotReady].
func LocalFacebookNotReady(stream string) bool {
	return sessions.LocalFacebookNotReady(stream)
}

// RecordLocalSession delegates to [sessions.Store.RecordLocalSession] via
// a.sessions (PR1 of the *AppStore dissolution, 2026-07-01).
func (a *AppStore) RecordLocalSession(ctx context.Context, accountID, orgID int64, status LocalSessionStatus, errMsg string) error {
	return a.sessions.RecordLocalSession(ctx, accountID, orgID, status, errMsg)
}
