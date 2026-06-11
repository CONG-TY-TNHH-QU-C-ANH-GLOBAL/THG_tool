package control_test

import (
	"testing"
	"time"

	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

func TestConnectPublicChannel(t *testing.T) {
	// Resolve succeeds → destination stored with the chat id/title Telegram returned.
	fs := &fakeSender{resChatID: -1009876, resTitle: "THG Channel", resUsername: "thgchan"}
	svc, st := newSvc(t, "tg_connect_pub.db", fs, control.Flags{NotifyEnabled: true})
	d, reason := svc.ConnectPublicChannel(7, 99, "thgchan")
	if d == nil || reason != "" || d.ChatID != -1009876 || d.Title != "THG Channel" {
		t.Fatalf("connect failed: d=%+v reason=%q", d, reason)
	}
	if act, _ := st.ListActiveDestinations(7); len(act) != 1 {
		t.Fatalf("expected 1 active destination, got %d", len(act))
	}

	// Resolve returns a Telegram 400 (channel/username invalid) → specific reason, nothing stored.
	failing := &fakeSender{} // resChatID 0 → SendResult{ErrCode:400}
	svc2, st2 := newSvc(t, "tg_connect_fail.db", failing, control.Flags{NotifyEnabled: true})
	if d, reason := svc2.ConnectPublicChannel(7, 99, "ghost"); d != nil || reason != "channel_not_found_or_username_invalid" {
		t.Fatalf("expected channel_not_found_or_username_invalid, got d=%+v reason=%q", d, reason)
	}
	if act, _ := st2.ListActiveDestinations(7); len(act) != 0 {
		t.Fatal("nothing should be stored on resolve failure")
	}
}

func TestChannelPostConnect(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_channelpost.db", fs, control.Flags{NotifyEnabled: true})
	if _, err := st.CreateBindCode(7, 99, "CHANCODE1", 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	// A channel post carrying the code connects the private channel to that org.
	svc.HandleChannelPost(-1005555, "Private Ops", "", "/connect CHANCODE1")
	act, _ := st.ListActiveDestinations(7)
	if len(act) != 1 || act[0].ChatID != -1005555 || act[0].Title != "Private Ops" {
		t.Fatalf("channel_post connect failed: %+v", act)
	}
	// A wrong/unknown code creates nothing (no tenant association).
	svc.HandleChannelPost(-1006666, "Other", "", "/connect WRONGGGG")
	if act, _ := st.ListActiveDestinations(7); len(act) != 1 {
		t.Fatal("unknown code must not create a destination")
	}
}

func TestTestDestination(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_dest_test.db", fs, control.Flags{NotifyEnabled: true})
	id, _ := st.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -100111})
	if ok, _ := svc.TestDestination(7, id); !ok || len(fs.sent) != 1 {
		t.Fatalf("test send should succeed, ok? sent=%d", len(fs.sent))
	}
	// Failing transport → records needs_attention.
	failing := &fakeSender{fail: true}
	svcF, stF := newSvc(t, "tg_dest_testfail.db", failing, control.Flags{NotifyEnabled: true})
	idF, _ := stF.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -100222})
	if ok, _ := svcF.TestDestination(7, idF); ok {
		t.Fatal("failing transport must report not-sent")
	}
	if d, _ := stF.GetDestination(7, idF); d.Status != "needs_attention" {
		t.Fatalf("failed test should set needs_attention: %+v", d)
	}
}

func TestNotifyEventRouting(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_notify_event.db", fs, control.Flags{NotifyEnabled: true})
	// Two destinations: one all-channels subscribed to lead_created, one taobao-only.
	_, _ = st.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -100, EventTypes: `["lead_created"]`, ChannelFilter: "all"})
	_, _ = st.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -200, EventTypes: `["lead_created"]`, ChannelFilter: "taobao"})
	// A different org's destination must NEVER receive org7's events.
	_, _ = st.UpsertDestination(tgstore.Destination{OrgID: 8, DestinationType: "channel", ChatID: -300, EventTypes: `["lead_created"]`, ChannelFilter: "all"})

	// facebook lead → only the all-channels destination (taobao filtered out, org8 isolated).
	if n, _ := svc.NotifyEvent(7, "lead_created", "facebook", "msg"); n != 1 {
		t.Fatalf("facebook lead_created should reach 1 destination, got %d", n)
	}
	// Unsubscribed event type → nobody.
	if n, _ := svc.NotifyEvent(7, "comment_failed", "facebook", "msg"); n != 0 {
		t.Fatal("unsubscribed event must reach nobody")
	}
	// Invalid event type → nobody.
	if n, _ := svc.NotifyEvent(7, "delete_all", "facebook", "msg"); n != 0 {
		t.Fatal("invalid event type must be rejected")
	}
	// Notify disabled globally → nobody.
	svcOff, stOff := newSvc(t, "tg_notify_off2.db", &fakeSender{}, control.Flags{NotifyEnabled: false})
	_, _ = stOff.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -1, EventTypes: `["lead_created"]`, ChannelFilter: "all"})
	if n, _ := svcOff.NotifyEvent(7, "lead_created", "facebook", "msg"); n != 0 {
		t.Fatal("notify disabled must send nothing")
	}
}

// Channel destinations and personal DM bindings COEXIST — destinations do not replace bindings.
func TestDestinationsAndBindingsCoexist(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_coexist.db", fs, control.Flags{NotifyEnabled: true})
	_, _ = st.UpsertDestination(tgstore.Destination{OrgID: 7, DestinationType: "channel", ChatID: -100, EventTypes: `["lead_created"]`, ChannelFilter: "all"})
	_, _ = st.CreateBindCode(7, 99, "DMCODE12", 30*time.Minute)
	_ = svc.HandleMessage(msg(555, 555, "/bind DMCODE12")) // personal DM binding still works
	if _, bound, _ := svc.ActiveBindings(555); !bound {
		t.Fatal("personal DM binding must still work alongside destinations")
	}
	if n, _ := svc.NotifyEvent(7, "lead_created", "facebook", "msg"); n != 1 {
		t.Fatal("channel destination still receives events independently of DM bindings")
	}
}
