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
	// Heat is the classification badge ("🔥 rất nóng"). Sale reads this first to
	// decide which lead to open, so it sits on line two.
	Heat string
	// ProductLine: "<tên> · 20 CNY · MOQ 2包" — mang giá thật của sàn.
	// ProductName một mình chưa bao giờ mang được giá.
	ProductLine string
	// ShippingLine: cước CRM tính từ biểu giá công bố ("$14.20 · 6–12 ngày").
	// ShippingBasis: tính từ đâu ra ("Epacket CN→US, 1 kiện 0.4 kg").
	ShippingLine, ShippingBasis string
	// CrmURL: trang CRM để mở lead. Rỗng thì bỏ dòng.
	CrmURL string
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

// Lead renders a "new lead" notification.
// indented renders a continuation line under the entry above it.
func indented(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "   " + strings.TrimSpace(value) + "\n"
}

// Lead renders the new-lead notice.
//
// Shape is deliberate and ordered by what Sale decides with, not by what is
// easiest to assemble:
//
//	who        name + heat — enough to decide whether to open at all
//	what       the post itself
//	numbers    product price, then landed cost with its basis
//	action     the reply they can send
//	links      where to go next
//
// The old shape led with "Workspace" and "Nguồn" — two fields that are the same
// on every single notice, so they pushed the post text below the fold and made
// every lead look identical in the list.
//
// Plain text: the bot sends without parse_mode, so no bold or italics. Emoji
// and line breaks carry the structure instead.
func Lead(m LeadMsg) string {
	excerpt := m.Excerpt
	if strings.TrimSpace(excerpt) == "" {
		excerpt = excerptFallback
	}
	var b strings.Builder

	b.WriteString("🔔 Lead mới")
	if author := strings.TrimSpace(m.Author); author != "" {
		b.WriteString(" · " + author)
	}
	b.WriteString("\n")

	// Heat, workspace and source on ONE line — all three answer "how do I triage
	// this", none of them is content. The old shape gave each its own labelled
	// line, which pushed the post text below the fold.
	//
	// Workspace stays even though a per-org bot means one group only ever sees
	// one workspace: this is a multi-tenant deployment, and the day two orgs
	// share a group is the day its absence becomes a silent mix-up.
	badge := []string{}
	for _, part := range []string{m.Heat, m.Workspace, m.SourceLabel} {
		if value := strings.TrimSpace(part); value != "" {
			badge = append(badge, value)
		}
	}
	if len(badge) > 0 {
		b.WriteString(strings.Join(badge, " · ") + "\n")
	}

	b.WriteString("\n" + strings.TrimSpace(excerpt) + "\n")

	if reason := strings.TrimSpace(m.Reason); reason != "" {
		b.WriteString("\n🎯 " + reason + "\n")
	}

	// Product: prefer the enriched one-liner (carries the real marketplace
	// price); fall back to the bare catalog name when no lookup happened.
	product := strings.TrimSpace(m.ProductLine)
	if product == "" {
		product = strings.TrimSpace(m.ProductName)
	}
	if product != "" {
		b.WriteString("\n🛍 Sản phẩm: " + product + "\n")
		b.WriteString(indented(m.ProductURL))
	}

	if ship := strings.TrimSpace(m.ShippingLine); ship != "" {
		b.WriteString("\n🚢 Cước tham chiếu: " + ship + "\n")
		b.WriteString(indented(m.ShippingBasis))
	}

	if reply := strings.TrimSpace(m.SuggestedReply); reply != "" {
		b.WriteString("\n💬 Gợi ý trả lời:\n" + reply + "\n")
	}

	b.WriteString(link("🔗 Bài gốc", m.PostURL))
	b.WriteString(link("📊 Mở CRM", firstNonEmpty(m.CrmURL, m.DashboardURL)))
	return tidy(b.String())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
