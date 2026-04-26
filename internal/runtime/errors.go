package runtime

import (
	"errors"
	"fmt"
)

// CDPErrorCode identifies the category of a CDP/browser failure.
type CDPErrorCode int

const (
	ErrChromeUnreachable  CDPErrorCode = iota + 1
	ErrNavigationTimeout
	ErrTabOpen
	ErrTabTTLExceeded
	ErrContentExtraction
	ErrSessionClosed
	ErrFacebookCheckpoint
	ErrFacebookLogout
	ErrFacebookBanned
	ErrNoSession
	ErrCooldownActive
	ErrRateLimitExceeded
)

func (c CDPErrorCode) String() string {
	switch c {
	case ErrChromeUnreachable:
		return "chrome_unreachable"
	case ErrNavigationTimeout:
		return "navigation_timeout"
	case ErrTabOpen:
		return "tab_open"
	case ErrTabTTLExceeded:
		return "tab_ttl_exceeded"
	case ErrContentExtraction:
		return "content_extraction"
	case ErrSessionClosed:
		return "session_closed"
	case ErrFacebookCheckpoint:
		return "facebook_checkpoint"
	case ErrFacebookLogout:
		return "facebook_logout"
	case ErrFacebookBanned:
		return "facebook_banned"
	case ErrNoSession:
		return "no_session"
	case ErrCooldownActive:
		return "cooldown_active"
	case ErrRateLimitExceeded:
		return "rate_limit_exceeded"
	default:
		return fmt.Sprintf("cdp_error_%d", int(c))
	}
}

// CDPError is the typed error returned by all runtime operations.
// Supports errors.Is() matching on Code alone.
type CDPError struct {
	Code    CDPErrorCode
	Message string
	Cause   error
}

func (e CDPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("cdp[%s]: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("cdp[%s]: %s", e.Code, e.Message)
}

func (e CDPError) Is(target error) bool {
	var t CDPError
	if errors.As(target, &t) {
		return t.Code == e.Code
	}
	return false
}

func (e CDPError) Unwrap() error { return e.Cause }

// Sentinel values for use with errors.Is().
var (
	ErrChrome     = CDPError{Code: ErrChromeUnreachable}
	ErrNavTimeout = CDPError{Code: ErrNavigationTimeout}
	ErrTab        = CDPError{Code: ErrTabOpen}
	ErrTTL        = CDPError{Code: ErrTabTTLExceeded}
	ErrExtract    = CDPError{Code: ErrContentExtraction}
	ErrCheckpoint = CDPError{Code: ErrFacebookCheckpoint}
	ErrBan        = CDPError{Code: ErrFacebookBanned}
	ErrNoSess     = CDPError{Code: ErrNoSession}
	ErrCooldown   = CDPError{Code: ErrCooldownActive}
)

var retryableCodes = map[CDPErrorCode]bool{
	ErrChromeUnreachable: true,
	ErrNavigationTimeout: true,
	ErrTabOpen:           true,
}

// IsRetryable reports whether err represents a transient failure worth retrying.
func IsRetryable(err error) bool {
	var e CDPError
	if errors.As(err, &e) {
		return retryableCodes[e.Code]
	}
	return false
}

// IsBanSignal reports whether err is a Facebook anti-scraping signal.
func IsBanSignal(err error) bool {
	var e CDPError
	if errors.As(err, &e) {
		return e.Code == ErrFacebookCheckpoint ||
			e.Code == ErrFacebookLogout ||
			e.Code == ErrFacebookBanned
	}
	return false
}

// Wrap wraps a raw error into a CDPError with the given code and message.
func Wrap(code CDPErrorCode, msg string, cause error) CDPError {
	return CDPError{Code: code, Message: msg, Cause: cause}
}
