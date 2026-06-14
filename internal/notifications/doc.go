// Package notifications delivers notifications (per-org Telegram channel, in-app
// bell) from durable events. It is a SINK, not a source of business logic: it
// renders a LeadCreated / CommentPosted event into a message and nothing more.
//
// Architecture role: NOTIFICATIONS (domain) — see MODULE_BOUNDARIES.md (notifications).
//
//   - Allowed imports (conceptual): models, telegram client, its store domain,
//     events (subscribe), stdlib.
//   - Forbidden imports (conceptual): services/facebook lead/comment logic,
//     outbound internals, connectors. Notifications must NOT own Facebook lead
//     logic — deciding whether a lead qualifies or queueing a comment is forbidden.
//
// SCAFFOLD ONLY (Phase A): boundary marker. Notification delivery currently lives in
// internal/telegram/control + internal/server/system/notifications.go and is fed by
// direct calls (tgEvents.NotifyLead) rather than by subscribing to durable events;
// it becomes an event subscriber in roadmap Phase E. See MODULE_OWNERSHIP.yml.
package notifications
