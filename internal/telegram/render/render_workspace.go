package render

import "strings"

// Workspace membership + extension lifecycle notifications
// (SaaS UX Hardening PR-8). Same plain-text grammar as the lead/action
// renderers: header, label lines, tidy().

// InviteCreated renders the admin-channel notice for a new invite.
func InviteCreated(workspace, email, role, expiresAt string) string {
	var b strings.Builder
	b.WriteString("✉️ Lời mời workspace mới\n\n")
	b.WriteString(line("Workspace", workspace))
	b.WriteString(line("Email được mời", email))
	b.WriteString(line("Vai trò", role))
	b.WriteString(line("Hết hạn", expiresAt))
	return tidy(b.String())
}

// InviteAccepted renders the admin-channel notice when a member joins.
func InviteAccepted(workspace, memberName, memberEmail, role string) string {
	var b strings.Builder
	b.WriteString("✅ Thành viên mới đã tham gia\n\n")
	b.WriteString(line("Workspace", workspace))
	b.WriteString(line("Thành viên", memberName))
	b.WriteString(line("Email", memberEmail))
	b.WriteString(line("Vai trò", role))
	return tidy(b.String())
}

// ExtensionUpdateRequired renders the rate-limited connector warning.
func ExtensionUpdateRequired(workspace, staffName, accountName, version, state string) string {
	var b strings.Builder
	b.WriteString("⚠️ Một connector cần cập nhật extension trước khi tiếp tục automation.\n\n")
	b.WriteString(line("Workspace", workspace))
	b.WriteString(line("Nhân viên", staffName))
	b.WriteString(line("Facebook account", accountName))
	b.WriteString(line("Phiên bản hiện tại", version))
	b.WriteString(line("Trạng thái", state))
	b.WriteString("\nAutomation tạm dừng cho account này đến khi extension được cập nhật.\n")
	return tidy(b.String())
}
