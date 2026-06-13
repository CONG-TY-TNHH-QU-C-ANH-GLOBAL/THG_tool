package connectors

import (
	"testing"
	"time"
)

// The resolver enforces the verification rule: the dashboard verifies the
// EXACT connector bound to one pairing session — never the latest workspace
// heartbeat, another user's connector, or another Chrome profile.
func TestResolvePairingFacebookStatus(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-10 * time.Second)
	stale := now.Add(-3 * time.Minute)
	codeAlive := now.Add(5 * time.Minute)
	codeDead := now.Add(-5 * time.Minute)
	session := &ConnectorPairingSession{ID: 7, OrgID: 1, CreatedBy: 100, DeviceTokenID: 55, Used: true, ExpiresAt: codeDead}

	token := func(mut func(*AgentToken)) *AgentToken {
		tok := &AgentToken{
			ID: 55, OrgID: 1, CreatedBy: 100, Active: true,
			StreamStatus: streamFacebookLoggedIn, FBUserID: "fb-1", LastSeen: &fresh,
		}
		if mut != nil {
			mut(tok)
		}
		return tok
	}

	cases := []struct {
		name    string
		session *ConnectorPairingSession
		token   *AgentToken
		ownerID int64
		want    PairingFacebookStatus
	}{
		{"unclaimed session", &ConnectorPairingSession{ID: 7, OrgID: 1, CreatedBy: 100, ExpiresAt: codeAlive}, nil, 0, PairingStatusWaitingPairing},
		{"unclaimed session past expiry", &ConnectorPairingSession{ID: 7, OrgID: 1, CreatedBy: 100, ExpiresAt: codeDead}, nil, 0, PairingStatusCodeExpired},
		{"binding removed via forget device", session, nil, 0, PairingStatusBindingReleased},
		{"revoked token", session, token(func(t *AgentToken) { t.Active = false }), 0, PairingStatusBindingReleased},
		{"another connector never verifies", session, token(func(t *AgentToken) { t.ID = 99 }), 0, PairingStatusWaitingPairing},
		{"another workspace never verifies", session, token(func(t *AgentToken) { t.OrgID = 2 }), 0, PairingStatusWaitingPairing},
		{"another user's heartbeat never verifies", session, token(func(t *AgentToken) { t.CreatedBy = 200 }), 0, PairingStatusWaitingPairing},
		{"fb account owned by another member", session, token(nil), 200, PairingStatusConflict},
		{"conflict outranks staleness while identity is current", session, token(func(t *AgentToken) { t.LastSeen = &stale }), 200, PairingStatusConflict},
		{"residual identity after logout is not a conflict", session, token(func(t *AgentToken) {
			t.StreamStatus = "facebook_login_required"
		}), 200, PairingStatusNotDetected},
		{"stale proof", session, token(func(t *AgentToken) { t.LastSeen = &stale }), 0, PairingStatusStale},
		{"no proof timestamp", session, token(func(t *AgentToken) { t.LastSeen = nil }), 0, PairingStatusStale},
		{"fresh but not logged in", session, token(func(t *AgentToken) {
			t.StreamStatus = "facebook_login_required"
			t.FBUserID = ""
		}), 0, PairingStatusNotDetected},
		{"fresh logged in", session, token(nil), 0, PairingStatusDetected},
		{"same owner reconnects same fb account", session, token(nil), 100, PairingStatusDetected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePairingFacebookStatus(tc.session, tc.token, tc.ownerID, now)
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}
