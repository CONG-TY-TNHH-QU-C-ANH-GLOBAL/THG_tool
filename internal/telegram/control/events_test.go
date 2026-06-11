package control_test

import (
	"strings"
	"testing"

	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

func dest(orgID, chatID int64, events string) tgstore.Destination {
	return tgstore.Destination{OrgID: orgID, DestinationType: "channel", ChatID: chatID, EventTypes: events, ChannelFilter: "all"}
}

func TestNotifyLead(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_evt_lead.db", fs, control.Flags{NotifyEnabled: true})
	_, _ = st.UpsertDestination(dest(7, -100, `["lead_created"]`))

	svc.NotifyLead(control.LeadNotice{
		OrgID: 7, LeadID: 55, Channel: "facebook", Workspace: "THG Fulfill",
		SourceName: "POD Fulfillment", Author: "Anonymous participant",
		Excerpt: "Đang tìm supplier cho mẫu đèn này, cần fulfill nhanh", Reason: "Tìm supplier / fulfillment",
		PostURL: "https://facebook.com/groups/1/posts/100", BaseURL: "https://sale.thgfulfill.com",
	})
	m := fs.last()
	for _, want := range []string{"Lead mới", "THG Fulfill", "POD Fulfillment", "Anonymous participant", "supplier", "facebook.com/groups/1/posts/100", "/leads/55"} {
		if !strings.Contains(m, want) {
			t.Fatalf("lead message missing %q:\n%s", want, m)
		}
	}
	// Garbage excerpt → fallback, NOT repeated Facebook.
	fs.sent = nil
	svc.NotifyLead(control.LeadNotice{OrgID: 7, LeadID: 1, Excerpt: "Facebook Facebook Facebook Facebook", BaseURL: "https://x.com"})
	if strings.Contains(fs.last(), "Facebook Facebook") || !strings.Contains(fs.last(), "Chưa có nội dung tóm tắt") {
		t.Fatalf("garbage excerpt not sanitized:\n%s", fs.last())
	}
	// Empty base URL → no dashboard line.
	fs.sent = nil
	svc.NotifyLead(control.LeadNotice{OrgID: 7, LeadID: 2, Excerpt: "tìm xưởng may áo thun", BaseURL: ""})
	if strings.Contains(fs.last(), "dashboard") {
		t.Fatalf("empty base must hide dashboard line:\n%s", fs.last())
	}
	// org 0 = no-op.
	fs.sent = nil
	svc.NotifyLead(control.LeadNotice{OrgID: 0})
	if len(fs.sent) != 0 {
		t.Fatal("org 0 must not deliver")
	}
}

func TestNotifyAction(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_evt_action.db", fs, control.Flags{NotifyEnabled: true})
	_, _ = st.UpsertDestination(dest(7, -100, `["comment_verified","comment_unverified","comment_failed"]`))

	base := control.ActionNotice{OrgID: 7, OutboundID: 9, Channel: "facebook", Workspace: "THG Fulfill",
		Agent: "Facebook David Anh", Author: "Anonymous participant", SourceName: "POD Fulfillment",
		CommentText: "Nếu bạn đang tìm supplier/fulfill cho mẫu đèn này...", PostURL: "https://fb/post", BaseURL: "https://sale.thgfulfill.com"}

	// verified → success header + verified status.
	n := base
	n.EventType = "comment_verified"
	svc.NotifyAction(n)
	if !strings.Contains(fs.last(), "Comment đã xuất hiện trên Facebook") || !strings.Contains(fs.last(), "David Anh") || !strings.Contains(fs.last(), "/outbox/9") {
		t.Fatalf("comment_verified render:\n%s", fs.last())
	}

	// unverified → informational, NOT a failure, has the manual-check hint.
	fs.sent = nil
	n.EventType = "comment_unverified"
	svc.NotifyAction(n)
	if !strings.Contains(fs.last(), "chưa xác minh được") || !strings.Contains(fs.last(), "Hành động:") || strings.Contains(fs.last(), "Thất bại") {
		t.Fatalf("comment_unverified must read informational:\n%s", fs.last())
	}

	// failed → reason + action hint.
	fs.sent = nil
	n.EventType = "comment_failed"
	n.Reason = "comment_box_not_found"
	svc.NotifyAction(n)
	if !strings.Contains(fs.last(), "chưa được gửi") || !strings.Contains(fs.last(), "comment_box_not_found") || !strings.Contains(fs.last(), "Hành động:") {
		t.Fatalf("comment_failed render:\n%s", fs.last())
	}

	// Not subscribed / invalid → nothing.
	fs.sent = nil
	n.EventType = "inbox_sent"
	svc.NotifyAction(n)
	n.EventType = "totally_bogus"
	svc.NotifyAction(n)
	if len(fs.sent) != 0 {
		t.Fatalf("unsubscribed/invalid must not deliver: %v", fs.sent)
	}
}
