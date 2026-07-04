// Domain: sessions (see internal/store/DOMAINS.md)
package store

import (
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
