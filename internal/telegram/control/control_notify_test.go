package control_test

import (
	"testing"

	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

func recipient(orgID, userID, tgUserID, chatID int64) tgstore.Binding {
	return tgstore.Binding{
		OrgID: orgID, UserID: userID, TelegramUserID: tgUserID, ChatID: chatID,
		AlertRecipient: true, Status: "active",
	}
}

func TestTestNotify(t *testing.T) {
	// notify disabled → typed reason, nothing sent.
	fs := &fakeSender{}
	svcOff, _ := newSvc(t, "tg_notify_off.db", fs, control.Flags{NotifyEnabled: false})
	if ok, reason := svcOff.TestNotify(7, 99); ok || reason != "notify_disabled" {
		t.Fatalf("disabled: ok=%v reason=%q", ok, reason)
	}

	// enabled + active binding with chat_id → delivered.
	fs2 := &fakeSender{}
	svc, st := newSvc(t, "tg_notify_on.db", fs2, control.Flags{NotifyEnabled: true})
	if _, err := st.UpsertBinding(recipient(7, 99, 555, 555)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.TestNotify(7, 99); !ok || len(fs2.sent) != 1 {
		t.Fatalf("expected delivery, ok=%v sent=%d", ok, len(fs2.sent))
	}

	// no binding → safe failure.
	if ok, reason := svc.TestNotify(7, 12345); ok || reason != "no_active_binding" {
		t.Fatalf("no binding: ok=%v reason=%q", ok, reason)
	}

	// sender error → handled safely (no panic, reported as no delivery).
	failing := &fakeSender{fail: true}
	svcFail, stF := newSvc(t, "tg_notify_fail.db", failing, control.Flags{NotifyEnabled: true})
	_, _ = stF.UpsertBinding(recipient(7, 99, 777, 777))
	if ok, _ := svcFail.TestNotify(7, 99); ok {
		t.Fatal("a failing sender must not report success")
	}
}

func TestNotifyBoundUsers(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_notify_alerts.db", fs, control.Flags{NotifyEnabled: true})
	if _, err := st.UpsertBinding(recipient(7, 99, 555, 555)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAlertPrefs(7, true, "all", `["connector_offline"]`); err != nil {
		t.Fatal(err)
	}

	// opted-in type + matching/any channel → delivered.
	if n, _ := svc.NotifyBoundUsers(7, "connector_offline", "facebook", "hi"); n != 1 {
		t.Fatalf("expected 1 delivery, got %d", n)
	}
	// not opted-in type → skipped.
	if n, _ := svc.NotifyBoundUsers(7, "gate1_failure_spike", "facebook", "hi"); n != 0 {
		t.Fatal("non-opted-in alert type must be skipped")
	}
	// channel-filtered org → skipped when channel differs.
	_ = st.UpsertAlertPrefs(7, true, "taobao", `["connector_offline"]`)
	if n, _ := svc.NotifyBoundUsers(7, "connector_offline", "facebook", "hi"); n != 0 {
		t.Fatal("channel filter mismatch must skip")
	}
	// alerts globally disabled in prefs → skipped.
	_ = st.UpsertAlertPrefs(7, false, "all", `["connector_offline"]`)
	if n, _ := svc.NotifyBoundUsers(7, "connector_offline", "facebook", "hi"); n != 0 {
		t.Fatal("alerts_enabled=false must skip")
	}
}
