package reels_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	serverreels "github.com/thg/scraper/internal/server/reels"
	reelsvc "github.com/thg/scraper/internal/services/reel"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func bootstrap(path string) error {
	db, err := store.New(path)
	if err != nil {
		return err
	}
	return db.Close()
}

func newApp(t *testing.T, name, secret string) *fiber.App {
	dst := storetest.CopyTemplate(t, bootstrap, name)
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := reelsvc.NewService(db, reelsvc.NewFakeRenderer(), func() *ai.MessageGenerator { return nil }, reelsvc.ScriptConfig{}, reelsvc.RenderConfig{})
	app := fiber.New()
	serverreels.WebhookRoutes(app.Group("/api"), serverreels.Deps{Service: svc, WebhookSecret: secret})
	return app
}

func sign(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func post(t *testing.T, app *fiber.App, body, sig string) int {
	req := httptest.NewRequest("POST", "/api/reel/webhook/render", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set("X-Reel-Signature", sig)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp.StatusCode
}

// TestRenderWebhookHMAC proves a bad signature is rejected (401) and a valid one is
// accepted (200). The handler is idempotent, so the unknown shot in the body is a no-op.
func TestRenderWebhookHMAC(t *testing.T) {
	const secret = "s3cret"
	app := newApp(t, "reel_wh.db", secret)
	body := `{"org_id":1,"reel_id":1,"scene":1,"provider_job_id":"fake_1_1","state":"done","output_key":"o/1.mp4","cost_usd":0}`

	if code := post(t, app, body, "deadbeef"); code != 401 {
		t.Fatalf("bad HMAC must 401, got %d", code)
	}
	if code := post(t, app, body, sign(body, secret)); code != 200 {
		t.Fatalf("valid HMAC must 200, got %d", code)
	}
}
