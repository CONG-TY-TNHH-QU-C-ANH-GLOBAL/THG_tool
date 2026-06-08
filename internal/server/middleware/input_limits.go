package middleware

import (
	"strings"

	"github.com/thg/scraper/internal/store/connectors"
)

// Connector heartbeat / screenshot / chrome-status payloads come from
// untrusted browser connectors. Without bounds the dashboard could be DOS'd
// by a buggy or malicious connector flooding 100KB strings into every
// presence row, bloating SQLite and slowing the connectors list endpoint.
//
// truncateRune limits a string to maxRunes runes (not bytes) so multi-byte
// Vietnamese names are not chopped mid-codepoint.
func TruncateRune(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	value = strings.TrimSpace(value)
	r := []rune(value)
	if len(r) <= maxRunes {
		return value
	}
	return string(r[:maxRunes])
}

// connectorInputLimits centralizes the upper bounds enforced on every
// connector-reported field. Values are deliberately generous so legitimate
// agents never get clipped, but tight enough to refuse abuse.
const (
	limConnectorHostname        = 128
	limConnectorOS              = 128
	limConnectorVersion         = 64
	limConnectorKind            = 64
	limConnectorTransport       = 64
	limConnectorCapabilitiesLen = 4096
	limConnectorURL             = 2048
	limConnectorFBUserID        = 64
	limConnectorFBName          = 255
	limConnectorFBUsername      = 80
	limConnectorFBProfileURL    = 512
	limConnectorLoginEmail      = 320
	limConnectorStreamStatus    = 64
	limConnectorChromeError     = 4096
	limConnectorIdentityMeta    = 64
)

// ClampPresenceFields enforces upper bounds on every connector-supplied field
// before agent presence is persisted.
func ClampPresenceFields(p *connectors.AgentPresence) {
	p.Hostname = TruncateRune(p.Hostname, limConnectorHostname)
	p.OS = TruncateRune(p.OS, limConnectorOS)
	p.Version = TruncateRune(p.Version, limConnectorVersion)
	p.Kind = TruncateRune(p.Kind, limConnectorKind)
	p.Transport = TruncateRune(p.Transport, limConnectorTransport)
	p.CapabilitiesJSON = TruncateRune(p.CapabilitiesJSON, limConnectorCapabilitiesLen)
	p.CurrentURL = TruncateRune(p.CurrentURL, limConnectorURL)
	p.FBUserID = TruncateRune(p.FBUserID, limConnectorFBUserID)
	p.FBDisplayName = TruncateRune(p.FBDisplayName, limConnectorFBName)
	p.FBUsername = TruncateRune(p.FBUsername, limConnectorFBUsername)
	p.FBProfileURL = TruncateRune(p.FBProfileURL, limConnectorFBProfileURL)
	p.StreamStatus = TruncateRune(p.StreamStatus, limConnectorStreamStatus)
	p.ChromeError = TruncateRune(p.ChromeError, limConnectorChromeError)
	p.IdentityConfidence = TruncateRune(p.IdentityConfidence, limConnectorIdentityMeta)
	p.IdentityExtractionMethod = TruncateRune(p.IdentityExtractionMethod, limConnectorIdentityMeta)
	p.IdentityLastVerifiedAt = TruncateRune(p.IdentityLastVerifiedAt, limConnectorIdentityMeta)
	p.BrowserProfileID = TruncateRune(p.BrowserProfileID, limConnectorIdentityMeta)
	p.MachineLabel = TruncateRune(p.MachineLabel, limConnectorFBName)
}
