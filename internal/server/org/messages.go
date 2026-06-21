package org

// HTTP error-response message strings shared across the org API handlers.
// Centralising each phrase here gives it a single definition (go:S1192)
// without growing the larger handler files. Values are unchanged from the
// inline literals they replace, so client-facing responses stay byte-identical.
const (
	msgWorkspaceContextRequired = "workspace context required"
	msgInvalidRequest           = "invalid request"
	msgUserNotFound             = "user not found"
	msgOrganizationNotFound     = "organization not found"
	msgNotFound                 = "not found"
)
