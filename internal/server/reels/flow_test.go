package reels_test

// End-to-end HTTP proof of the reel backend — the 10-step "Postman flow" driven through
// the real auth middleware + a minted JWT (RENDER_PROVIDER=fake, zero cost). Mirrors the
// repo's httptest idiom (see webhook_test.go). bootstrap() and sign() are shared from
// webhook_test.go (same package).

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/ai"
	authpkg "github.com/thg/scraper/internal/auth"
	serverreels "github.com/thg/scraper/internal/server/reels"
	reelsvc "github.com/thg/scraper/internal/services/reel"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/storetest"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

const flowSecret = "s3cret"
const jwtSecret = "test-jwt-secret"

type flowEnv struct {
	app *fiber.App
	db  *store.Store
	tok string // org 1 admin
	tok2 string // org 2 admin (tenant-isolation probe)
}

func newFlow(t *testing.T) *flowEnv {
	dst := storetest.CopyTemplate(t, bootstrap, "reel_flow.db")
	db, err := store.New(dst)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	svc := reelsvc.NewService(db, reelsvc.NewFakeRenderer(), func() *ai.MessageGenerator { return nil }, reelsvc.ScriptConfig{}, reelsvc.RenderConfig{})
	deps := serverreels.Deps{Service: svc, WebhookSecret: flowSecret}

	// Mirror production wiring (router.go): webhook on the unauthenticated base /api
	// group; reel endpoints on a child group carrying RequireAuth.
	app := fiber.New()
	api := app.Group("/api")
	serverreels.WebhookRoutes(api, deps)
	protected := api.Group("", authpkg.RequireAuth(jwtSecret))
	serverreels.Routes(protected, deps)

	t1, _ := authpkg.GenerateAccessToken(1, 1, "a@t", "admin", jwtSecret)
	t2, _ := authpkg.GenerateAccessToken(2, 2, "b@t", "admin", jwtSecret)
	return &flowEnv{app: app, db: db, tok: t1, tok2: t2}
}

func (e *flowEnv) do(t *testing.T, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// TestReelFlow walks the 10-step Postman proof end to end.
func TestReelFlow(t *testing.T) {
	e := newFlow(t)

	// 1) Create reel → 201, script_ready, has shot_list.
	code, body := e.do(t, "POST", "/api/reels", e.tok,
		`{"brief_style":"seller ship chậm mất khách","keywords":["BUNG","fulfill US"],"target_duration_sec":25}`)
	if code != 201 {
		t.Fatalf("step1 want 201 got %d (%v)", code, body)
	}
	script := body["script"].(map[string]any)
	if script["shot_list"] == nil || script["shot_list"] == "[]" {
		t.Fatalf("step1 missing shot_list: %v", script)
	}
	reelID := int64(body["reel"].(map[string]any)["id"].(float64))

	// 2) GET → script_ready.
	code, body = e.do(t, "GET", "/api/reels/"+itoa(reelID), e.tok, "")
	if code != 200 || body["status"] != "script_ready" {
		t.Fatalf("step2 want 200 script_ready got %d %v", code, body["status"])
	}

	// 3) PATCH script → version 2.
	code, body = e.do(t, "PATCH", "/api/reels/"+itoa(reelID)+"/script", e.tok, `{"caption":"v2 caption"}`)
	if code != 200 {
		t.Fatalf("step3 want 200 got %d", code)
	}
	if v := body["script"].(map[string]any)["version"].(float64); v != 2 {
		t.Fatalf("step3 want version 2 got %v", v)
	}

	// 4) Approve → rendering, shots present.
	code, body = e.do(t, "POST", "/api/reels/"+itoa(reelID)+"/approve", e.tok, "")
	if code != 200 || body["status"] != "rendering" {
		t.Fatalf("step4 want 200 rendering got %d %v", code, body["status"])
	}
	shots := body["shots"].([]any)
	if len(shots) == 0 {
		t.Fatal("step4 expected shots")
	}
	nShots := len(shots)

	// 5) Approve again → idempotent: NO new shots.
	code, body = e.do(t, "POST", "/api/reels/"+itoa(reelID)+"/approve", e.tok, "")
	if code != 200 {
		t.Fatalf("step5 want 200 got %d", code)
	}
	if got := len(body["shots"].([]any)); got != nShots {
		t.Fatalf("step5 idempotency broken: shots %d -> %d", nShots, got)
	}

	// 6) Webhook (fake) per scene with valid HMAC → done, cost accrues.
	for _, s := range shots {
		sh := s.(map[string]any)
		pj := sh["provider_job_id"].(string)
		scene := int64(sh["scene"].(float64))
		wb := `{"org_id":1,"reel_id":` + itoa(reelID) + `,"scene":` + itoa(scene) +
			`,"provider_job_id":"` + pj + `","state":"done","output_key":"o/` + pj + `.mp4","cost_usd":0.06}`
		c := post(t, e.app, wb, sign(wb, flowSecret))
		if c != 200 {
			t.Fatalf("step6 webhook scene %d want 200 got %d", scene, c)
		}
	}

	// 7) GET → all shots done, cost summed, ready to post.
	code, body = e.do(t, "GET", "/api/reels/"+itoa(reelID), e.tok, "")
	total := int(body["shots_total"].(float64))
	done := int(body["shots_done"].(float64))
	if code != 200 || total == 0 || done != total {
		t.Fatalf("step7 want all done got total=%d done=%d", total, done)
	}
	if cost := body["total_cost_usd"].(float64); cost <= 0 {
		t.Fatalf("step7 expected accrued cost got %v", cost)
	}

	// 8) Webhook bad HMAC → 401.
	if c := post(t, e.app, `{"org_id":1,"reel_id":1}`, "deadbeef"); c != 401 {
		t.Fatalf("step8 bad HMAC want 401 got %d", c)
	}

	// 9) Publish → outbound id; verify post_reel row carries the video.
	code, body = e.do(t, "POST", "/api/reels/"+itoa(reelID)+"/publish", e.tok,
		`{"account_id":10,"target_url":"https://facebook.com/me"}`)
	if code != 200 || body["allowed"] != true {
		t.Fatalf("step9 want 200 allowed got %d %v", code, body)
	}
	obID := int64(body["outbound_id"].(float64))
	if obID <= 0 {
		t.Fatalf("step9 expected outbound_id got %d", obID)
	}
	var typ, mediaType string
	if err := e.db.DB().QueryRow(
		`SELECT type, media_type FROM outbound_messages WHERE id = ? AND org_id = ?`, obID, 1,
	).Scan(&typ, &mediaType); err != nil {
		t.Fatalf("step9 read outbound: %v", err)
	}
	if typ != "post_reel" || mediaType != "video" {
		t.Fatalf("step9 want post_reel/video got %s/%s", typ, mediaType)
	}

	// 10) Cross-org GET → 404 (tenant isolation).
	code, _ = e.do(t, "GET", "/api/reels/"+itoa(reelID), e.tok2, "")
	if code != 404 {
		t.Fatalf("step10 cross-org want 404 got %d", code)
	}
}
