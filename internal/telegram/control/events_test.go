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

func TestNotifyLeadCreated(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_evt_lead.db", fs, control.Flags{NotifyEnabled: true})
	_, _ = st.UpsertDestination(dest(7, -100, `["lead_created"]`))

	svc.NotifyLeadCreated(7, 55, "THG Fulfill", "Facebook group", "Anonymous", "đang tìm supplier cho mẫu đèn", "https://app.example.com")
	if len(fs.sent) != 1 || !strings.Contains(fs.last(), "Lead mới") || !strings.Contains(fs.last(), "Anonymous") {
		t.Fatalf("lead notification not delivered/rendered: %q", fs.last())
	}
	// A nil/zero-org call is a safe no-op.
	svc.NotifyLeadCreated(0, 1, "", "", "", "", "")
	if len(fs.sent) != 1 {
		t.Fatal("org 0 must not deliver")
	}
}

func TestNotifyAgentAction(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_evt_action.db", fs, control.Flags{NotifyEnabled: true})
	_, _ = st.UpsertDestination(dest(7, -100, `["comment_verified","comment_failed"]`))

	// Success → AgentComment render.
	svc.NotifyAgentAction(7, "comment_verified", "facebook", "David Anh", "Anonymous", "đã xác minh", "https://fb/post", "https://app")
	if len(fs.sent) != 1 || !strings.Contains(fs.last(), "Agent đã gửi comment") {
		t.Fatalf("comment_verified not rendered: %q", fs.last())
	}
	// Failure event → Failure render.
	fs.sent = nil
	svc.NotifyAgentAction(7, "comment_failed", "facebook", "David Anh", "Anonymous", "thất bại", "", "https://app")
	if len(fs.sent) != 1 || !strings.Contains(fs.last(), "Cần kiểm tra") {
		t.Fatalf("comment_failed not rendered: %q", fs.last())
	}
	// Not subscribed (destination only opted into comment_verified/failed) → nothing.
	fs.sent = nil
	svc.NotifyAgentAction(7, "inbox_sent", "facebook", "David Anh", "Anonymous", "x", "", "")
	if len(fs.sent) != 0 {
		t.Fatal("unsubscribed event must not deliver")
	}
	// Invalid event type → nothing.
	svc.NotifyAgentAction(7, "totally_bogus", "facebook", "a", "b", "c", "", "")
	if len(fs.sent) != 0 {
		t.Fatal("invalid event type must be rejected")
	}
}
