// Runtime regression for the Telegram control service: command parsing, the no-execution policy,
// the /bind consume flow, and the /status·/unbind·unknown dispatch. DB-backed via the shared
// storetest template + a fake Sender that records replies.
package control_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
	tgstore "github.com/thg/scraper/internal/store/telegram"
	"github.com/thg/scraper/internal/telegram/control"
)

type fakeSender struct {
	sent        []string
	fail        bool
	resChatID   int64 // canned Resolve response (connect-public-channel tests)
	resTitle    string
	resUsername string
}

func (f *fakeSender) Send(_ int64, text string) error {
	if f.fail {
		return errors.New("boom")
	}
	f.sent = append(f.sent, text)
	return nil
}
func (f *fakeSender) Resolve(_, _ string) (int64, string, string, error) {
	if f.fail || f.resChatID == 0 {
		return 0, "", "", errors.New("resolve failed")
	}
	f.sent = append(f.sent, "[resolve]")
	return f.resChatID, f.resTitle, f.resUsername, nil
}
func (f *fakeSender) last() string {
	if len(f.sent) == 0 {
		return ""
	}
	return f.sent[len(f.sent)-1]
}

func bootstrap(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newSvc(t *testing.T, name string, s control.Bot, flags control.Flags) (*control.Service, *tgstore.Store) {
	dst := storetest.CopyTemplate(t, bootstrap, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := db.Telegram()
	return control.NewService(st, s, flags), st
}

func msg(tg, chat int64, text string) control.IncomingMessage {
	return control.IncomingMessage{TgUserID: tg, ChatID: chat, FirstName: "Sale", Text: text}
}

// ParseCommand + the no-execution policy are pure.
func TestParseAndPolicy(t *testing.T) {
	cmd, arg := control.ParseCommand("/bind ABC123")
	if cmd != "bind" || arg != "ABC123" {
		t.Fatalf("parse = %q,%q", cmd, arg)
	}
	if c, _ := control.ParseCommand("/status@MyBot"); c != "status" {
		t.Fatalf("@bot suffix not stripped: %q", c)
	}
	if c, _ := control.ParseCommand("hello"); c != "" {
		t.Fatal("non-command must parse empty")
	}
	if !control.IsExecutionCommand("comment") || !control.IsExecutionCommand("auto_comment") {
		t.Fatal("execution commands must be flagged")
	}
	if control.IsExecutionCommand("status") {
		t.Fatal("status is not an execution command")
	}
}

func TestBindFlow(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_ctrl_bind.db", fs, control.Flags{})
	if _, err := st.CreateBindCode(7, 99, "GOODCODE", 10*time.Minute); err != nil {
		t.Fatal(err)
	}

	// invalid code → error, no binding.
	_ = svc.HandleMessage(msg(555, 555, "/bind NOPE"))
	if !strings.Contains(fs.last(), "không hợp lệ") {
		t.Fatalf("expected bind error, got %q", fs.last())
	}

	// valid code → success + active binding with chat_id.
	_ = svc.HandleMessage(msg(555, 555, "/bind goodcode")) // lower-case tolerated
	if !strings.Contains(fs.last(), "thành công") {
		t.Fatalf("expected bind success, got %q", fs.last())
	}
	bs, bound, _ := svc.ActiveBindings(555)
	if !bound || bs[0].OrgID != 7 || bs[0].ChatID != 555 {
		t.Fatalf("binding not created correctly: %+v", bs)
	}

	// reused code → error (single use).
	_ = svc.HandleMessage(msg(556, 556, "/bind GOODCODE"))
	if !strings.Contains(fs.last(), "không hợp lệ") {
		t.Fatal("reused code must fail")
	}

	// expired code → error.
	_, _ = st.CreateBindCode(7, 99, "EXPIRED", -1*time.Minute)
	_ = svc.HandleMessage(msg(557, 557, "/bind EXPIRED"))
	if !strings.Contains(fs.last(), "không hợp lệ") {
		t.Fatal("expired code must fail")
	}
}

func TestDispatch(t *testing.T) {
	fs := &fakeSender{}
	svc, st := newSvc(t, "tg_ctrl_dispatch.db", fs, control.Flags{})

	// /start and /help are valid while unbound.
	_ = svc.HandleMessage(msg(1, 1, "/start"))
	_ = svc.HandleMessage(msg(1, 1, "/help"))
	// /status unbound → info, not a crash.
	_ = svc.HandleMessage(msg(1, 1, "/status"))
	if !strings.Contains(fs.last(), "chưa được liên kết") {
		t.Fatalf("unbound status: %q", fs.last())
	}
	// unknown command → help fallback.
	_ = svc.HandleMessage(msg(1, 1, "/wat"))
	if !strings.Contains(fs.last(), "Không nhận ra lệnh") {
		t.Fatalf("unknown should fall back to help: %q", fs.last())
	}
	// execution command → denied (actions disabled).
	_ = svc.HandleMessage(msg(1, 1, "/comment xyz"))
	if !strings.Contains(fs.last(), "TẮT") {
		t.Fatalf("execution must be denied: %q", fs.last())
	}

	// Bind, then /status bound updates last_command_at, /unbind revokes.
	_, _ = st.CreateBindCode(7, 99, "BINDME11", 10*time.Minute)
	_ = svc.HandleMessage(msg(2, 2, "/bind BINDME11"))
	_ = svc.HandleMessage(msg(2, 2, "/status"))
	if !strings.Contains(fs.last(), "Đã liên kết") {
		t.Fatalf("bound status: %q", fs.last())
	}
	bs, _, _ := svc.ActiveBindings(2)
	if !bs[0].LastCommandAt.Valid {
		t.Fatal("last_command_at must be stamped after a bound command")
	}
	_ = svc.HandleMessage(msg(2, 2, "/unbind"))
	if _, bound, _ := svc.ActiveBindings(2); bound {
		t.Fatal("/unbind must revoke the binding")
	}
}
