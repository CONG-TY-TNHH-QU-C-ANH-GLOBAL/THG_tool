package models

import "time"

// Notification type vocabulary (closed set — deterministic boundary).
// Consumers branch on these exact values; the frontend maps them to
// cards/copy. Phase 1 of the notification substrate carries only the
// invite flow + extension warnings (SaaS UX Hardening PR-1/PR-4).
const (
	NotificationInviteReceived          = "workspace_invite_received"
	NotificationInviteAccepted          = "workspace_invite_accepted"
	NotificationWorkspaceJoined         = "workspace_joined"
	NotificationExtensionUpdateRequired = "extension_update_required"
)

// Notification is the contract shape for one in-app notification.
//
// Scoping rule (mirrors the store queries):
//   - UserID > 0 → personal: visible to that user only, regardless of
//     the user's current org (an invite reaches the invitee BEFORE they
//     join the inviting org — OrgID names the org the event concerns).
//   - UserID = 0 → org-wide: visible to admins of OrgID.
type Notification struct {
	ID          int64      `json:"id"`
	OrgID       int64      `json:"org_id"`
	UserID      int64      `json:"user_id"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	PayloadJSON string     `json:"payload_json"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
