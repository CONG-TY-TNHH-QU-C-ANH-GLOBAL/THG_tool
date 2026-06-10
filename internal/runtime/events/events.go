// Package events is the typed lifecycle-event taxonomy for THG's
// runtime. It is the emission contract paired with the Runtime
// Topology doc (specs/RUNTIME_TOPOLOGY.md §5 — Failure Surface +
// §6 — CI Enforcement).
//
// Why a typed taxonomy: without one, slog event names drift across
// emission sites — "exec-verify: attempt classified" here,
// "execution.verified" there, "verified attempt" somewhere else.
// The Runtime Control Plane (project_runtime_control_plane) needs a
// stable event vocabulary so the runtime-feed dashboard can render
// without parsing free-form log strings.
//
// Design:
//
//   - Constants for every event name. Emission sites import this
//     package and reference the constant — never a raw string.
//   - Constants for the structured field keys so org_id is always
//     `org_id` and never `orgID` / `org`.
//   - Two helpers: [Info] for success-class events, [Warn] for
//     failure-class events. Both prepend `event=<name>` to the slog
//     attribute list.
//
// What this package is NOT:
//
//   - It is NOT an event bus or publish/subscribe layer. It is
//     emission-only. Consumers parse the slog stream the standard way
//     ([[feedback_freeze_abstraction]] — no new abstractions).
//   - It is NOT a schema definition for an `events` DB table. Schema
//     persistence is a separate concern tracked under EXP-1 of
//     project_runtime_control_plane.
//   - It is NOT a wrapper around slog. Callers may still use slog
//     directly for non-runtime events (HTTP access logs, dev debug).
//     This package is for the runtime topology event surface only.
package events

// Event name constants. Format: `<domain>.<verb>` so the dashboard can
// group by domain prefix. Add new constants in alphabetical order by
// domain prefix to keep diffs reviewable.
const (
	// Crawl / ingest events.
	CrawlerURLRepair  = "crawler.url_repair"
	CrawlerIngestSkip = "crawler.ingest_skip"

	// Outbound state-machine events.
	OutboundQueued        = "outbound.queued"
	OutboundClaimed       = "outbound.claimed"
	OutboundFinalized     = "outbound.finalized"
	OutboundQueueRejected = "outbound.queue_rejected"

	// Execution verification events (extension proof → backend classifier).
	ExecutionAttemptBegun = "execution.attempt_begun"
	ExecutionVerified     = "execution.verified"
	ExecutionHookFailed   = "execution.hook_failed"

	// Coordination domain events.
	EngagementReconcile = "engagement.reconcile"
	EngagementRevoked   = "engagement.revoked"

	// Lead lifecycle events.
	LeadArchiveSweep = "lead.archive_sweep"

	// Risk / behaviour events.
	RiskSignalApplied = "risk.signal_applied"
	RiskCooldownSet   = "risk.cooldown_set"
)

// Field key constants. Use these instead of raw strings so dashboards
// can rely on stable JSON keys.
const (
	FieldEvent      = "event"
	FieldOrgID      = "org_id"
	FieldAccountID  = "account_id"
	FieldOutboundID = "outbound_id"
	FieldAttemptID  = "attempt_id"
	FieldTargetURL  = "target_url"
	FieldActionType = "action_type"
	FieldOutcome    = "outcome"
	FieldReason     = "reason"
	FieldHook       = "hook"
	FieldErr        = "error"
	// Maintenance / sweep metrics.
	FieldScanned    = "scanned_count"
	FieldArchived   = "archived_count"
	FieldReasons    = "archive_reason_counts"
	FieldDurationMS = "duration_ms"
)
