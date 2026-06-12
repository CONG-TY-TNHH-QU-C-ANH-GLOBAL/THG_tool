// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import (
	"strconv"
	"strings"
)

// Facebook Automation Reliability Track — the SINGLE connector-eligibility
// evaluator, shared by the create-time mission preflight (server/crawl) and the
// run-time connector picker (cmd/scraper) so the two NEVER diverge. See
// specs/FACEBOOK_AUTOMATION_RELIABILITY_TRACK.md (PR-A point 4).

// MinExtensionVersion is the lowest Chrome-extension version allowed to run an
// automation action. One source of truth; bump when an automation-breaking fix
// ships.
const MinExtensionVersion = "0.5.26"

// streamFacebookLoggedIn mirrors browsergateway.StreamFacebookLoggedIn. Kept as a
// literal so the connectors store does not import the gateway package.
const streamFacebookLoggedIn = "facebook_logged_in"

// Connector-eligibility reason codes (closed set). The version gate's
// blocking reasons (ConnExtensionUpdateRequired / ConnExtensionUnsupported)
// live in version_policy.go next to the state evaluator.
const (
	ConnReady            = "ready"
	ConnOffline          = "connector_offline"
	ConnIdentityUnknown  = "actor_identity_unknown"
	ConnIdentityMismatch = "actor_mismatch"
	// Deprecated: pre-PR-4 single-floor reason, superseded by
	// ConnExtensionUpdateRequired. Kept for log/telemetry consumers.
	ConnExtensionOutdated = "extension_version_outdated"
)

// PickReadyConnector classifies whether any connector in conns can run an action
// for accountID. PURE over the connector list. Returns the chosen connector id
// (>0) with reason ConnReady, or 0 + a typed reason for the first online
// connector's blocking condition, else ConnOffline.
//
//   - a connector bound to a DIFFERENT account is ignored.
//   - the connector must be Online and StreamStatus == facebook_logged_in.
//   - its live fb_user_id must be present (else ConnIdentityUnknown) and, when
//     the account has an expected fb_user_id, must match it (else
//     ConnIdentityMismatch).
//   - the extension version state must allow automation (PR-4): blocked
//     states map to ConnExtensionUpdateRequired / ConnExtensionUnsupported.
func PickReadyConnector(conns []AgentToken, accountID int64, expectedFBUserID string, policy VersionPolicy) (int64, string) {
	expected := strings.TrimSpace(expectedFBUserID)
	sawAssigned := false
	for i := range conns {
		c := conns[i]
		if c.AssignedAccountID > 0 && c.AssignedAccountID != accountID {
			continue
		}
		sawAssigned = true
		if !c.Online {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(c.StreamStatus), streamFacebookLoggedIn) {
			continue
		}
		connFB := strings.TrimSpace(c.FBUserID)
		if connFB == "" {
			return 0, ConnIdentityUnknown
		}
		if expected != "" && connFB != expected {
			return 0, ConnIdentityMismatch
		}
		if state := EvaluateVersionState(c.Version, policy); !VersionStateAllowsAutomation(state) {
			return 0, VersionStateReason(state)
		}
		return c.ID, ConnReady
	}
	_ = sawAssigned
	return 0, ConnOffline
}

// versionAtLeast compares dotted version strings numerically segment by segment
// ("0.5.29.878" >= "0.5.26" → true). Missing trailing segments count as 0.
func versionAtLeast(v, min string) bool {
	va := strings.Split(strings.TrimSpace(v), ".")
	vb := strings.Split(strings.TrimSpace(min), ".")
	for i := 0; i < len(vb); i++ {
		bPart, _ := strconv.Atoi(vb[i])
		aPart := 0
		if i < len(va) {
			aPart, _ = strconv.Atoi(va[i])
		}
		if aPart != bPart {
			return aPart > bPart
		}
	}
	return true
}
