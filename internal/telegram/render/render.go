// Package render builds the Telegram bot's reply text. Leaf package — imports nothing, holds no
// business logic and no sensitive data (never renders tokens, codes, or another tenant's data).
// Vietnamese-primary, concise and operator-friendly. The control layer composes these from
// already-authorised primitives.
package render

import "fmt"

// Start / Help — onboarding + the control-plane command list. Execution is explicitly absent.
func Help() string {
	return "🤖 *THG Telegram*\n\n" +
		"Các lệnh hỗ trợ:\n" +
		"/bind <mã> — liên kết tài khoản (lấy mã trong Cài đặt → Tích hợp → Telegram)\n" +
		"/status — xem trạng thái liên kết\n" +
		"/unbind — hủy liên kết\n" +
		"/help — trợ giúp\n\n" +
		"Telegram chỉ dùng để theo dõi & cảnh báo. Không thực thi hành động (gửi bình luận) qua Telegram."
}

func Start() string {
	return "👋 Chào mừng đến với THG.\n\n" + Help()
}

// BindSuccess confirms a binding. name is the operator's display name (no codes/tokens echoed).
func BindSuccess(name string) string {
	if name == "" {
		name = "bạn"
	}
	return fmt.Sprintf("✅ Liên kết thành công, %s! Bạn sẽ nhận cảnh báo tại đây. Dùng /status để kiểm tra.", name)
}

// BindError — a generic, safe failure (never reveals whether the code existed).
func BindError() string {
	return "❌ Mã không hợp lệ hoặc đã hết hạn. Vào Cài đặt → Tích hợp → Telegram để tạo mã mới rồi gửi /bind <mã>."
}

// StatusUnbound / StatusBound — connection status for the requesting account.
func StatusUnbound() string {
	return "ℹ️ Tài khoản Telegram này chưa được liên kết. Gửi /bind <mã> để liên kết."
}

func StatusBound(orgCount int) string {
	return fmt.Sprintf("✅ Đã liên kết với %d workspace. Bạn đang nhận cảnh báo control-plane. Gửi /unbind để hủy.", orgCount)
}

// Unbind outcomes.
func UnbindDone(n int) string {
	return fmt.Sprintf("✅ Đã hủy liên kết (%d). Gửi /bind <mã> để liên kết lại.", n)
}

func UnbindNone() string {
	return "ℹ️ Không có liên kết nào để hủy."
}

// Denied — an outbound-execution command was attempted. Telegram never executes actions.
func Denied() string {
	return "🚫 Thực thi hành động qua Telegram đang TẮT. Telegram chỉ hỗ trợ thiết lập, trạng thái và cảnh báo."
}

// Unknown — any unrecognised input falls back to help safely.
func Unknown() string {
	return "Không nhận ra lệnh. " + Help()
}

// TestMessage is the body of the "send test notification" action from the web app.
func TestMessage() string {
	return "🔔 Đây là thông báo thử từ THG. Nếu bạn nhận được tin này, kênh cảnh báo Telegram đang hoạt động."
}

// ChannelConnected confirms a channel is linked to the workspace (sent into the channel on connect).
func ChannelConnected() string {
	return "✅ Channel đã được kết nối với THG AutoFlow. Các thông báo vận hành sẽ được gửi vào đây."
}

// ── Event message builders (used by emitters + tests; channel-neutral, no secrets) ──

// LeadCreated renders the "new lead" notification.
func LeadCreated(workspace, source, lead, summary, dashboardURL string) string {
	return "📌 Lead mới từ " + orDash(source) + "\n" +
		"Workspace: " + orDash(workspace) + "\n" +
		"Lead: " + orDash(lead) + "\n" +
		"Tóm tắt: " + orDash(summary) + "\n" +
		"Trạng thái: sẵn sàng xử lý\n" +
		"Mở dashboard: " + dashboardURL
}

// AgentComment renders the "agent submitted a comment" notification. state e.g. "đã gửi" / "chờ xác minh".
func AgentComment(channel, account, lead, state, postURL, dashboardURL string) string {
	return "✅ Agent đã gửi comment\n" +
		"Channel: " + orDash(channel) + "\n" +
		"Account: " + orDash(account) + "\n" +
		"Lead: " + orDash(lead) + "\n" +
		"Trạng thái: " + orDash(state) + "\n" +
		"Mở post: " + postURL + "\n" +
		"Mở dashboard: " + dashboardURL
}

// Failure renders an attention/failure notification. reason is a typed reason code.
func Failure(channel, account, reason, dashboardURL string) string {
	return "⚠️ Cần kiểm tra\n" +
		orDash(channel) + " " + orDash(account) + " gặp lỗi.\n" +
		"Lý do: " + orDash(reason) + "\n" +
		"Mở dashboard: " + dashboardURL
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
