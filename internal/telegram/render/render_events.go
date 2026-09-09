package render

import "strings"

// Plain-text Telegram notifications (no MarkdownV2 — avoids escaping bugs with _ ( ) and links).
// Empty fields/links are omitted so a message never shows "Mở dashboard:" with no URL. Mobile-
// readable: emoji header → key facts → excerpt/comment block → status → links → action hint.

func line(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return label + ": " + value + "\n"
}

func block(label, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "\n" + label + ":\n" + value + "\n"
}

func link(label, url string) string {
	if strings.TrimSpace(url) == "" {
		return ""
	}
	return "\n" + label + ":\n" + url + "\n"
}

// excerptFallback: shown when the post has no usable text content.
const excerptFallback = "Chưa có nội dung tóm tắt. Mở bài viết để xem chi tiết."

// LeadMsg / ActionMsg carry the already-resolved + sanitized fields the control layer assembled.
type LeadMsg struct {
	Workspace, SourceLabel, Author, Excerpt, Reason, Status, PostURL, DashboardURL string
	// SuggestedReply/ProductName/ProductURL/ProductImageURL are the optional operator-facing reply
	// suggestion (generated upstream; the sink only renders it). Empty = omitted.
	SuggestedReply, ProductName, ProductURL, ProductImageURL string
	// Supplier* describe a sourceable 1688 / Taobao item matched for this lead.
	// SupplierSummary is the one-line facts strip (price · weight · MOQ · origin);
	// it is empty when the marketplace published none of those numbers.
	SupplierPlatform, SupplierName, SupplierURL, SupplierSummary string
}

type ActionMsg struct {
	Header, Workspace, Agent, Author, SourceName, CommentText, Status, Reason, Hint, PostURL, OutboxURL string
}

func tidy(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimRight(s, "\n")
}

// supplierLabel names the marketplace inline when it is known, so the operator
// can tell a 1688 wholesale offer from a Taobao retail listing at a glance.
func supplierLabel(platform string) string {
	if platform = strings.TrimSpace(platform); platform != "" {
		return "🏭 Nguồn hàng " + platform
	}
	return "🏭 Nguồn hàng"
}

// Lead renders a "new lead" notification.
func Lead(m LeadMsg) string {
	excerpt := m.Excerpt
	if strings.TrimSpace(excerpt) == "" {
		excerpt = excerptFallback
	}
	var b strings.Builder
	b.WriteString("📌 Lead mới từ Facebook\n\n")
	b.WriteString(line("Workspace", m.Workspace))
	b.WriteString(line("Nguồn", m.SourceLabel))
	b.WriteString(line("Người đăng", m.Author))
	b.WriteString(block("Nội dung", "\""+excerpt+"\""))
	b.WriteString(block("Lý do phù hợp", m.Reason))
	b.WriteString(block("💬 Gợi ý trả lời", m.SuggestedReply))
	b.WriteString(line("🛍 Sản phẩm gợi ý", m.ProductName))
	b.WriteString(link("🔗 Link sản phẩm", m.ProductURL))
	b.WriteString(link("🖼 Ảnh sản phẩm", m.ProductImageURL))
	b.WriteString(line(supplierLabel(m.SupplierPlatform), m.SupplierName))
	b.WriteString(line("📦 Thông số nguồn", m.SupplierSummary))
	b.WriteString(link("🔗 Link nguồn hàng", m.SupplierURL))
	b.WriteString(line("Trạng thái", m.Status))
	b.WriteString(link("🔗 Mở bài viết Facebook", m.PostURL))
	b.WriteString(link("📊 Mở trong dashboard", m.DashboardURL))
	return tidy(b.String())
}

// Action renders a comment/inbox/post outcome notification.
func Action(m ActionMsg) string {
	var b strings.Builder
	b.WriteString(m.Header + "\n\n")
	b.WriteString(line("Workspace", m.Workspace))
	b.WriteString(line("Agent", m.Agent))
	b.WriteString(line("Bài viết của", m.Author))
	b.WriteString(line("Nguồn", m.SourceName))
	b.WriteString(block("Comment đã gửi", quoteIf(m.CommentText)))
	b.WriteString(line("Trạng thái", m.Status))
	b.WriteString(line("Lý do", m.Reason))
	if strings.TrimSpace(m.Hint) != "" {
		b.WriteString("Hành động: " + m.Hint + "\n")
	}
	b.WriteString(link("🔗 Mở bài viết", m.PostURL))
	b.WriteString(link("📊 Mở outbox", m.OutboxURL))
	return tidy(b.String())
}

func quoteIf(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return "\"" + s + "\""
}
