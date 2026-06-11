package control

import (
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/telegram/render"
)

// High-level notification emitters used by the automation emitters (crawler, outbox agent). The
// CALLER provides resolved business data (workspace name, agent account name, post URL, raw
// excerpt); control sanitizes the excerpt, builds dashboard/outbox URLs, renders, and routes via
// NotifyEvent to the org's channel destinations. All nil-safe + best-effort — a notification
// failure NEVER affects the crawl/comment path. No secrets/chat_id/token ever travel here.

// ── URL builders (centralized; empty base → empty link → renderer hides the line) ──
func baseTrim(base string) string { return strings.TrimRight(strings.TrimSpace(base), "/") }

func dashboardLeadURL(base string, leadID int64) string {
	b := baseTrim(base)
	if b == "" {
		return ""
	}
	if leadID > 0 {
		return b + "/leads/" + strconv.FormatInt(leadID, 10)
	}
	return b + "/leads"
}

func outboxURL(base string, outboundID int64) string {
	b := baseTrim(base)
	if b == "" {
		return ""
	}
	if outboundID > 0 {
		return b + "/outbox/" + strconv.FormatInt(outboundID, 10)
	}
	return b + "/outbox"
}

// sourceLabel composes "Nhóm Facebook — <name>" when the source name is known, else the generic.
func sourceLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Nhóm Facebook"
	}
	return "Nhóm Facebook — " + strings.TrimSpace(name)
}

// LeadNotice is the raw lead data a caller hands to NotifyLead.
type LeadNotice struct {
	OrgID, LeadID                                   int64
	Channel, Workspace, SourceName, Author, PostURL string
	Excerpt, Reason, BaseURL                        string
}

// NotifyLead emits a rich "new lead" channel notification.
func (s *Service) NotifyLead(n LeadNotice) {
	if s == nil || n.OrgID == 0 {
		return
	}
	channel := n.Channel
	if channel == "" {
		channel = "facebook"
	}
	msg := render.Lead(render.LeadMsg{
		Workspace:    n.Workspace,
		SourceLabel:  sourceLabel(n.SourceName),
		Author:       n.Author,
		Excerpt:      SanitizeExcerpt(n.Excerpt),
		Reason:       strings.TrimSpace(n.Reason),
		Status:       "Sẵn sàng xử lý",
		PostURL:      strings.TrimSpace(n.PostURL),
		DashboardURL: dashboardLeadURL(n.BaseURL, n.LeadID),
	})
	_, _ = s.NotifyEvent(n.OrgID, "lead_created", channel, msg)
}

// action presentation: header + status + failure flag + human-action hint, per event type. Future
// post_*/inbox_* are included so the renderer is ready when those emitters wire up.
type actionView struct {
	header, status, hint string
	failure              bool
}

var actionPresentation = map[string]actionView{
	"comment_submitted":  {"✅ Agent đã gửi comment", "Đã gửi, đang chờ xác minh", "", false},
	"comment_verified":   {"✅ Comment đã xuất hiện trên Facebook", "Đã xác minh", "", false},
	"comment_unverified": {"ℹ️ Comment đã gửi nhưng chưa xác minh được", "Đã gửi, chưa xác minh", "Mở bài viết hoặc xác nhận thủ công nếu thấy comment.", false},
	"comment_failed":     {"⚠️ Comment chưa được gửi", "Thất bại", "Mở dashboard để retry hoặc kiểm tra tab Facebook.", true},
	"inbox_sent":         {"✅ Đã gửi inbox", "Đã gửi", "", false},
	"inbox_failed":       {"⚠️ Gửi inbox thất bại", "Thất bại", "Mở dashboard để kiểm tra.", true},
	"post_submitted":     {"✅ Đã đăng bài", "Đã đăng", "", false},
	"post_failed":        {"⚠️ Đăng bài thất bại", "Thất bại", "Mở dashboard để kiểm tra.", true},
}

// ActionNotice is the raw agent-action data a caller hands to NotifyAction.
type ActionNotice struct {
	OrgID, OutboundID                                        int64
	EventType, Channel, Workspace, Agent, Author, SourceName string
	CommentText, PostURL, Reason, BaseURL                    string
}

// NotifyAction emits a rich comment/inbox/post outcome channel notification.
func (s *Service) NotifyAction(n ActionNotice) {
	if s == nil || n.OrgID == 0 || !IsValidEventType(n.EventType) {
		return
	}
	v, ok := actionPresentation[n.EventType]
	if !ok {
		return
	}
	channel := n.Channel
	if channel == "" {
		channel = "facebook"
	}
	comment := ""
	if strings.HasPrefix(n.EventType, "comment") {
		comment = n.CommentText
	}
	reason := ""
	if v.failure {
		reason = strings.TrimSpace(n.Reason)
	}
	msg := render.Action(render.ActionMsg{
		Header: v.header, Workspace: n.Workspace, Agent: n.Agent, Author: n.Author,
		SourceName: strings.TrimSpace(n.SourceName), CommentText: comment, Status: v.status,
		Reason: reason, Hint: v.hint, PostURL: strings.TrimSpace(n.PostURL),
		OutboxURL: outboxURL(n.BaseURL, n.OutboundID),
	})
	_, _ = s.NotifyEvent(n.OrgID, n.EventType, channel, msg)
}
