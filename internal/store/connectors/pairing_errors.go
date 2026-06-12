// Domain: connectors (see internal/store/DOMAINS.md)
package connectors

import "errors"

// Typed pairing errors. The HTTP layer maps these to stable error_code strings
// so the extension popup can show a specific Vietnamese message instead of the
// generic "invalid code" catch-all. Sentinels (not string matching) per the
// deterministic-boundaries rule: callers branch with errors.Is.
var (
	// Messages keep the legacy wording (invalid / already used / expired) so
	// pre-0.5.55 extensions, which regex-match the body, still show their
	// generic Vietnamese fallback.
	ErrPairingCodeInvalid  = errors.New("invalid pairing code")
	ErrPairingCodeConsumed = errors.New("pairing code already used")
	ErrPairingCodeExpired  = errors.New("pairing code expired")

	// The Chrome profile (stable extension_installation_id) is already bound
	// to a different THG user / a different workspace. Pairing must not
	// silently re-bind a profile across the ownership boundary; the operator
	// must Forget Device (or use another Chrome profile) first.
	ErrDevicePairedToAnotherUser      = errors.New("device instance already paired to another user")
	ErrDevicePairedToAnotherWorkspace = errors.New("device instance already paired to another workspace")
)

// PairingErrorCode returns the stable machine-readable code for a typed
// pairing error, or "" when err is not a pairing error.
func PairingErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPairingCodeInvalid):
		return "pairing_code_invalid"
	case errors.Is(err, ErrPairingCodeConsumed):
		return "pairing_code_consumed"
	case errors.Is(err, ErrPairingCodeExpired):
		return "pairing_code_expired"
	case errors.Is(err, ErrDevicePairedToAnotherUser):
		return "device_instance_already_paired_to_another_user"
	case errors.Is(err, ErrDevicePairedToAnotherWorkspace):
		return "device_instance_already_paired_to_another_workspace"
	default:
		return ""
	}
}

// ClaimedPairing is the result of a successful pairing-code claim. The
// PairingSessionID lets the dashboard verify the Facebook login of THIS
// pairing only (never "latest workspace heartbeat").
type ClaimedPairing struct {
	Token            *AgentToken
	DeviceToken      string
	PairingSessionID int64
}
