// Package control is the SINGLE SOURCE OF TRUTH for the Telegram integration's domain rules:
// the command registry, the action-execution policy, audit-event names, the alert-type and
// channel-filter allow-lists, binding/ownership rules, and the binding/command/notification
// services. The REST settings API (internal/server/integrations) and the webhook runtime
// (internal/server/telegram) BOTH depend on this package and never re-implement these rules.
//
// policy.go holds the pure, dependency-free rules (no DB, no HTTP, no Telegram API).
package control

import "strings"

// Control-plane commands the bot understands. There is intentionally NO outbound-execution
// command (comment/post/send) — Telegram is control-plane only.
const (
	CmdStart  = "start"
	CmdHelp   = "help"
	CmdBind   = "bind"
	CmdStatus = "status"
	CmdUnbind = "unbind"
)

// SupportedCommands is the canonical control-plane command set.
var SupportedCommands = []string{CmdStart, CmdHelp, CmdBind, CmdStatus, CmdUnbind}

// executionCommands are explicitly DENIED: they would execute an outbound action, which Telegram
// must never do (TELEGRAM_ACTIONS_ENABLED is off and there is no execution path here at all).
var executionCommands = map[string]bool{
	"comment": true, "send": true, "auto_comment": true, "autocomment": true,
	"post": true, "reply": true, "execute": true,
}

// IsExecutionCommand reports whether a command name is an outbound-execution attempt.
func IsExecutionCommand(cmd string) bool { return executionCommands[strings.ToLower(cmd)] }

// IsSupported reports whether a command is a known control-plane command.
func IsSupported(cmd string) bool {
	for _, c := range SupportedCommands {
		if c == cmd {
			return true
		}
	}
	return false
}

// ParseCommand extracts (cmd, arg) from a Telegram message. Handles "/bind ABC123",
// "/status@MyBot", leading whitespace, and case. Returns ("","") for non-command text.
func ParseCommand(text string) (cmd, arg string) {
	t := strings.TrimSpace(text)
	if !strings.HasPrefix(t, "/") {
		return "", ""
	}
	parts := strings.Fields(t[1:])
	if len(parts) == 0 {
		return "", ""
	}
	cmd = strings.ToLower(parts[0])
	if at := strings.IndexByte(cmd, '@'); at >= 0 { // strip @botname suffix
		cmd = cmd[:at]
	}
	if len(parts) > 1 {
		arg = parts[1]
	}
	return cmd, arg
}

// ── Audit event names (the ONLY place these strings are defined) ──
const (
	AuditBindCodeGenerated    = "bind_code_generated"
	AuditBindSuccess          = "bind_success"
	AuditBindFailed           = "bind_failed"
	AuditUnbind               = "unbind"
	AuditBindingRevoked       = "binding_revoked"
	AuditIntegrationEnabled   = "integration_enabled"
	AuditIntegrationDisabled  = "integration_disabled"
	AuditTestNotification     = "test_notification"
	AuditAlertsUpdated        = "alerts_updated"
	AuditCommandReceived      = "command_received"
	AuditCommandDenied        = "command_denied"
	AuditNotificationSent     = "notification_sent"
	AuditNotificationFailed   = "notification_failed"
	AuditDestinationConnected = "destination_connected"
	AuditDestinationDisabled  = "destination_disabled"
	AuditDestinationTest      = "destination_test"
	AuditDestinationPrefs     = "destination_prefs_updated"
	AuditBotSaved             = "bot_token_saved"
	AuditBotVerified          = "bot_token_verified"
	AuditBotRevoked           = "bot_token_revoked"
)

// ── Allow-lists (single source of truth; REST API + UI both mirror these) ──

// AlertTypes is the channel-neutral set of alert kinds the product supports.
var AlertTypes = []string{
	"connector_offline", "gate1_failure_spike", "submitted_unverified_spike",
	"automation_paused", "account_needs_attention", "circuit_breaker_triggered",
}

// ChannelFilters is the channel-neutral filter allow-list (Facebook now; Taobao/1688 modelled).
var ChannelFilters = []string{"all", "facebook", "taobao", "1688"}

// EventTypes is the full set of automation events a notification destination can subscribe to
// (superset of the legacy system-health AlertTypes). Channel-neutral. Lead / agent-action /
// system-health categories. This is the SINGLE source of truth the REST API + UI mirror.
var EventTypes = []string{
	// lead lifecycle
	"lead_created", "lead_assigned", "lead_ready_for_review",
	// agent actions
	"comment_submitted", "comment_verified", "comment_unverified", "comment_failed",
	"post_submitted", "post_failed", "inbox_sent", "inbox_failed",
	// system / health
	"connector_offline", "account_attention", "automation_paused",
	"gate1_failure_spike", "submitted_unverified_spike", "circuit_breaker_triggered",
	// workspace membership + extension lifecycle (SaaS UX Hardening PR-8)
	"invite_created", "invite_accepted", "extension_update_required",
}

// IsValidEventType validates one event key against the allow-list.
func IsValidEventType(t string) bool { return inList(t, EventTypes) }

// SanitizeEventTypes drops any unknown event type (defends the preferences payload).
func SanitizeEventTypes(types []string) []string {
	out := make([]string, 0, len(types))
	for _, t := range types {
		if IsValidEventType(t) {
			out = append(out, t)
		}
	}
	return out
}

func inList(v string, list []string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// IsValidAlertType / IsValidChannelFilter validate a single value against the allow-lists.
func IsValidAlertType(t string) bool     { return inList(t, AlertTypes) }
func IsValidChannelFilter(f string) bool { return inList(f, ChannelFilters) }

// SanitizeAlertTypes drops any unknown alert type (defends the PUT payload).
func SanitizeAlertTypes(types []string) []string {
	out := make([]string, 0, len(types))
	for _, t := range types {
		if IsValidAlertType(t) {
			out = append(out, t)
		}
	}
	return out
}

// NormalizeChannelFilter returns a valid filter, defaulting to "all".
func NormalizeChannelFilter(f string) string {
	if f == "" || !IsValidChannelFilter(f) {
		return "all"
	}
	return f
}
