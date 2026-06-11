// Transport regression for the Telegram webhook: secret validation, always-200 on parsed updates,
// and that a parsed message reaches the shared control service. Business logic is covered by the
// control package tests; here we only prove the thin transport + secret gate.
package telegram_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	servertelegram "github.com/thg/scraper/internal/server/telegram"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
	"github.com/thg/scraper/internal/telegram/control"
)

type capSender struct{ n int }

func (c *capSender) Send(_ int64, _ string) error { c.n++; return nil }
func (c *capSender) Resolve(_, _ string) (control.SendResult, error) {
	return control.SendResult{}, nil
}
func (c *capSender) GetMe() (control.BotInfo, error) {
	return control.BotInfo{BotID: 1, Username: "webhookbot"}, nil
}

func bootstrap(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newApp(t *testing.T, name, secret string, sender control.Bot) (*fiber.App, *store.Store) {
	dst := storetest.CopyTemplate(t, bootstrap, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	factory := func(string) control.Bot { return sender }
	svc := control.NewService(db.Telegram(), factory, control.Flags{WebhookSecret: secret, GlobalToken: "webhookbot"})
	app := fiber.New()
	servertelegram.Routes(app.Group("/api"), servertelegram.Deps{Service: svc, WebhookSecret: secret})
	return app, db
}

func post(t *testing.T, app *fiber.App, body, secret string) int {
	req := httptest.NewRequest("POST", "/api/telegram/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp.StatusCode
}

func TestWebhookSecret(t *testing.T) {
	cs := &capSender{}
	app, _ := newApp(t, "tg_wh_secret.db", "s3cret", cs)
	body := `{"message":{"text":"/help","from":{"id":1,"first_name":"A"},"chat":{"id":1}}}`

	if code := post(t, app, body, "wrong"); code != 401 {
		t.Fatalf("wrong secret must 401, got %d", code)
	}
	if cs.n != 0 {
		t.Fatal("a rejected webhook must not reach the service")
	}
	if code := post(t, app, body, "s3cret"); code != 200 {
		t.Fatalf("valid secret must 200, got %d", code)
	}
	if cs.n != 1 {
		t.Fatalf("valid webhook must dispatch once, got %d", cs.n)
	}
}

func TestWebhookDispatchBind(t *testing.T) {
	cs := &capSender{}
	app, db := newApp(t, "tg_wh_bind.db", "", cs) // no secret configured
	if _, err := db.Telegram().CreateBindCode(7, 99, "WEBHOOK1", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	body := `{"message":{"text":"/bind WEBHOOK1","from":{"id":42,"first_name":"Op"},"chat":{"id":42}}}`
	if code := post(t, app, body, ""); code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	bs, err := db.Telegram().GetActiveBindingsByTelegramUser(42)
	if err != nil || len(bs) != 1 || bs[0].OrgID != 7 || bs[0].ChatID != 42 {
		t.Fatalf("webhook /bind did not create the binding via control: %+v err=%v", bs, err)
	}
}
