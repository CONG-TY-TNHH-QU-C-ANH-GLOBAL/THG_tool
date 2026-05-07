package recovery

import (
	"strings"

	"github.com/thg/scraper/internal/textutil"
)

// BanSignal classifies what kind of account impairment was detected.
type BanSignal int

const (
	SignalNone         BanSignal = iota
	SignalLoginFailure           // session expired / logged out
	SignalCheckpoint             // Facebook identity verification required
	SignalCaptcha               // CAPTCHA challenge
	SignalRateLimit             // temporary rate-limit / "try again later"
	SignalDisabled              // account permanently disabled
)

func (s BanSignal) String() string {
	switch s {
	case SignalLoginFailure:
		return "login_failure"
	case SignalCheckpoint:
		return "checkpoint"
	case SignalCaptcha:
		return "captcha"
	case SignalRateLimit:
		return "rate_limit"
	case SignalDisabled:
		return "disabled"
	default:
		return "none"
	}
}

// ShouldRotate returns true when switching to another account temporarily is the right response.
func (s BanSignal) ShouldRotate() bool {
	return s == SignalRateLimit || s == SignalCaptcha
}

// ShouldReAuth returns true when the session needs to be re-established via login.
func (s BanSignal) ShouldReAuth() bool {
	return s == SignalLoginFailure || s == SignalCheckpoint
}

// IsFatal returns true when the account is permanently unusable.
func (s BanSignal) IsFatal() bool {
	return s == SignalDisabled
}

// Detector inspects page state (title + URL + body snippet) to classify impairment.
type Detector struct{}

func New() *Detector { return &Detector{} }

// Analyze classifies the page described by title, url, and a content snippet.
// content should be the first ~2 KB of visible page text for performance.
func (d *Detector) Analyze(title, url, content string) BanSignal {
	t := strings.ToLower(title)
	u := strings.ToLower(url)
	c := strings.ToLower(content)
	srcs := []string{t, c}

	// Permanent disable takes priority
	if matchAny(srcs, "your account has been disabled", "tài khoản của bạn đã bị vô hiệu hóa") {
		return SignalDisabled
	}

	// Checkpoint / identity confirmation
	if textutil.ContainsAny(u, "checkpoint", "identity", "xác minh") ||
		matchAny(srcs, "confirm your identity", "xác nhận danh tính", "security check") {
		return SignalCheckpoint
	}

	// CAPTCHA
	if textutil.ContainsAny(u, "captcha") ||
		matchAny(srcs, "captcha", "i'm not a robot", "tôi không phải robot", "robot check") {
		return SignalCaptcha
	}

	// Login / session expired
	if textutil.ContainsAny(u, "login", "loginfb", "?next=") ||
		matchAny(srcs,
			"log in to facebook", "đăng nhập vào facebook",
			"you must log in", "session expired",
			"you've been logged out") {
		return SignalLoginFailure
	}

	// Rate limit / temporary block
	if matchAny(srcs,
		"you're temporarily blocked",
		"tạm thời bị chặn",
		"try again later",
		"thử lại sau",
		"action blocked",
		"hành động bị chặn",
		"you're doing this too fast") {
		return SignalRateLimit
	}

	return SignalNone
}

// matchAny reports whether any pattern appears in any of the sources.
func matchAny(sources []string, patterns ...string) bool {
	for _, p := range patterns {
		for _, s := range sources {
			if strings.Contains(s, p) {
				return true
			}
		}
	}
	return false
}

