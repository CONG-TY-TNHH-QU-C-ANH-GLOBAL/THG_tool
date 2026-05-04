package store

import (
	"context"
	"strings"
	"time"
)

// LocalSessionStatus is the typed enum stored in browser_sessions.status for
// rows that represent a Facebook account browser owned by a local connector
// (THG Local Runtime / desktop bridge) or a Docker workspace container.
//
// Centralizing these strings prevents typos like "local_starging" silently
// flipping a session into an unknown bucket, and lets the dashboard reason
// about the session lifecycle uniformly across the workspace and connector
// code paths.
type LocalSessionStatus string

const (
	SessionStarting     LocalSessionStatus = "local_starting"
	SessionActive       LocalSessionStatus = "local_active"
	SessionReady        LocalSessionStatus = "local_ready"
	SessionLoginReq     LocalSessionStatus = "local_login_required"
	SessionHumanReq     LocalSessionStatus = "local_human_required"
	SessionError        LocalSessionStatus = "local_error"
	SessionTerminated   LocalSessionStatus = "local_terminated"
	SessionInitializing LocalSessionStatus = "initializing"
	SessionDisplayReady LocalSessionStatus = "display_ready"
	SessionCheckpoint   LocalSessionStatus = "checkpoint"
	SessionIdle         LocalSessionStatus = "idle"
	SessionErrorState   LocalSessionStatus = "error"
)

// LocalSessionStatusFromStream maps the connector-reported stream status (the
// strings emitted by THG Local Runtime in /api/agent/heartbeat,
// /api/agent/screenshot, /api/agent/chrome-status) to the canonical
// browser_sessions.status value used by the dashboard.
//
// Streams are reported in lower-case kebab-style; we normalise here so the
// rest of the code only has to compare against the typed enum.
func LocalSessionStatusFromStream(stream string) LocalSessionStatus {
	switch strings.ToLower(strings.TrimSpace(stream)) {
	case "facebook_logged_in":
		return SessionReady
	case "facebook_human_required":
		return SessionHumanReq
	case "facebook_login_required":
		return SessionLoginReq
	case "chrome_not_connected":
		return SessionStarting
	default:
		return SessionActive
	}
}

// LocalFacebookNotReady returns true when the connector stream indicates the
// Facebook session is unusable (login wall, checkpoint, or Chrome detached).
// Callers use this to clear the cached browser_logged_in flag on the account.
func LocalFacebookNotReady(stream string) bool {
	switch strings.ToLower(strings.TrimSpace(stream)) {
	case "facebook_login_required", "facebook_human_required", "chrome_not_connected":
		return true
	default:
		return false
	}
}

// RecordLocalSession upserts a browser_sessions row for an account that is
// owned by a local runtime (no Docker container ports). It is the single
// entry point handlers should call instead of constructing BrowserSession
// values manually so the schema stays consistent.
func (a *AppStore) RecordLocalSession(ctx context.Context, accountID, orgID int64, status LocalSessionStatus, errMsg string) error {
	now := time.Now().UTC()
	return a.UpsertSession(ctx, BrowserSession{
		AccountID:    accountID,
		OrgID:        orgID,
		Status:       string(status),
		StartedAt:    now,
		LastActiveAt: now,
		ErrorMsg:     errMsg,
	})
}
