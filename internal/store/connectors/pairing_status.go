// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import (
	"database/sql"
	"time"
)

// ConnectorPairingSession is the dashboard-visible state of one pairing code:
// who created it, in which workspace, and which connector token (if any)
// claimed it. It is the exact-match anchor for Facebook login verification —
// the dashboard verifies THIS session's connector, never "latest heartbeat".
type ConnectorPairingSession struct {
	ID            int64
	OrgID         int64
	CreatedBy     int64
	DeviceTokenID int64
	Used          bool
	ExpiresAt     time.Time
}

// GetConnectorPairingSession loads one pairing session scoped to the org.
// Returns (nil, nil) when the session does not exist in this workspace.
func (s *Store) GetConnectorPairingSession(id, orgID int64) (*ConnectorPairingSession, error) {
	var sess ConnectorPairingSession
	var usedAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT id, org_id, created_by, COALESCE(device_token_id, 0), used_at, expires_at
		 FROM connector_pairing_codes
		 WHERE id = ? AND org_id = ?`,
		id, orgID,
	).Scan(&sess.ID, &sess.OrgID, &sess.CreatedBy, &sess.DeviceTokenID, &usedAt, &sess.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.Used = usedAt.Valid
	return &sess, nil
}

// PairingFacebookStatus is the typed verification outcome for one pairing
// session. Values are stable API strings consumed by the dashboard wizard.
type PairingFacebookStatus string

const (
	// PairingStatusWaitingPairing — no active connector claimed this code yet
	// (not claimed, or the binding was removed via Forget Device).
	PairingStatusWaitingPairing PairingFacebookStatus = "waiting_pairing"
	// PairingStatusDetected — the exact paired connector reported a fresh
	// logged-in Facebook session.
	PairingStatusDetected PairingFacebookStatus = "detected"
	// PairingStatusStale — the exact paired connector exists but its last
	// proof is older than the freshness window.
	PairingStatusStale PairingFacebookStatus = "facebook_session_stale"
	// PairingStatusNotDetected — fresh proof from the exact connector, but no
	// logged-in Facebook session on that Chrome profile.
	PairingStatusNotDetected PairingFacebookStatus = "facebook_session_not_detected"
	// PairingStatusConflict — the Facebook account seen on this connector is
	// already connected by another workspace member.
	PairingStatusConflict PairingFacebookStatus = "facebook_account_already_connected_to_another_member"
	// PairingStatusCodeExpired — the code expired before any extension
	// claimed it; the operator must generate a new one.
	PairingStatusCodeExpired PairingFacebookStatus = "pairing_code_expired"
	// PairingStatusBindingReleased — the code WAS claimed but the binding has
	// since been released (Forget Device / dashboard disconnect). The code is
	// consumed; re-pasting it cannot work — the operator needs a new one.
	PairingStatusBindingReleased PairingFacebookStatus = "binding_released"
)

// PairingProofFreshness is the window within which a connector heartbeat
// counts as live proof. Heartbeats fire every ~30s, so 60s tolerates exactly
// one missed beat (spec: 30–60s).
const PairingProofFreshness = 60 * time.Second

// ResolvePairingFacebookStatus derives the verification outcome for one
// pairing session from the EXACT connector token bound to it. Pure function —
// callers load the rows; this never reads "latest workspace heartbeat".
//
// token MUST be the agent_tokens row with ID == session.DeviceTokenID (nil
// when unclaimed or revoked). accountOwnerID is accounts.assigned_user_id for
// the token's live fb_user_id (0 when unknown/unassigned).
func ResolvePairingFacebookStatus(session *ConnectorPairingSession, token *AgentToken, accountOwnerID int64, now time.Time) PairingFacebookStatus {
	if session == nil {
		return PairingStatusWaitingPairing
	}
	if session.DeviceTokenID <= 0 {
		if !session.Used && now.After(session.ExpiresAt) {
			return PairingStatusCodeExpired
		}
		return PairingStatusWaitingPairing
	}
	if token == nil || !token.Active {
		return PairingStatusBindingReleased
	}
	// Exact-match doctrine: wrong token, wrong workspace, or wrong owner is a
	// caller bug — never silently verify against it.
	if token.ID != session.DeviceTokenID || token.OrgID != session.OrgID || token.CreatedBy != session.CreatedBy {
		return PairingStatusWaitingPairing
	}
	loggedIn := token.StreamStatus == streamFacebookLoggedIn && token.FBUserID != ""
	// Conflict only counts when the connector CURRENTLY reports the identity
	// as logged in — a residual fb_user_id after logout (heartbeats preserve
	// last-known identity) must resolve to not_detected, not conflict.
	if loggedIn && accountOwnerID != 0 && accountOwnerID != session.CreatedBy {
		return PairingStatusConflict
	}
	if token.LastSeen == nil || now.Sub(*token.LastSeen) > PairingProofFreshness {
		return PairingStatusStale
	}
	if loggedIn {
		return PairingStatusDetected
	}
	return PairingStatusNotDetected
}
