// Domain: sessions (see internal/store/DOMAINS.md)
package store

import (
	"context"

	"github.com/thg/scraper/internal/store/sessions"
)

// BrowserSession is an alias of [sessions.BrowserSession] for source
// compatibility. New code should import "internal/store/sessions" and use
// [sessions.BrowserSession] directly.
type BrowserSession = sessions.BrowserSession

// --- *AppStore bridge methods (PR1 of the *AppStore dissolution, 2026-07-01) ---
//
// These delegate to a.sessions (the sessions-domain subpackage). They stay
// because ~10 external callers still construct *AppStore via NewAppStore(db)
// and call these methods directly; migrate callers incrementally to
// db.Sessions().<Method>() and retire these wrappers once the last caller
// moves (mirrors the outbound_aliases.go L2 pattern).

// UpsertSession delegates to [sessions.Store.UpsertSession].
func (a *AppStore) UpsertSession(ctx context.Context, s BrowserSession) error {
	return a.sessions.UpsertSession(ctx, s)
}

// GetSession delegates to [sessions.Store.GetSession].
func (a *AppStore) GetSession(ctx context.Context, accountID int64) (*BrowserSession, error) {
	return a.sessions.GetSession(ctx, accountID)
}

// ListAllActiveSessions delegates to [sessions.Store.ListAllActiveSessions].
func (a *AppStore) ListAllActiveSessions(ctx context.Context) ([]BrowserSession, error) {
	return a.sessions.ListAllActiveSessions(ctx)
}

// ListSessions delegates to [sessions.Store.ListSessions].
func (a *AppStore) ListSessions(ctx context.Context, orgID int64) ([]BrowserSession, error) {
	return a.sessions.ListSessions(ctx, orgID)
}

// GetFirstActiveCDPSession delegates to [sessions.Store.GetFirstActiveCDPSession].
func (a *AppStore) GetFirstActiveCDPSession(ctx context.Context) (*BrowserSession, error) {
	return a.sessions.GetFirstActiveCDPSession(ctx)
}

// TerminateSession delegates to [sessions.Store.TerminateSession].
func (a *AppStore) TerminateSession(ctx context.Context, accountID int64) error {
	return a.sessions.TerminateSession(ctx, accountID)
}
