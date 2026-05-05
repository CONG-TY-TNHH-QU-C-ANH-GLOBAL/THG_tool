package server

import "strings"

// Connector heartbeat / screenshot / chrome-status payloads come from
// untrusted browser connectors. Without bounds the dashboard could be DOS'd
// by a buggy or malicious connector flooding 100KB strings into every
// presence row, bloating SQLite and slowing the connectors list endpoint.
//
// truncateRune limits a string to maxRunes runes (not bytes) so multi-byte
// Vietnamese names are not chopped mid-codepoint.
func truncateRune(value string, maxRunes int) string {
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
)
