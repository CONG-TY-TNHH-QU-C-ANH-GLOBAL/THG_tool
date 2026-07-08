// End-to-end Reel API workflow tests. The reel store is Postgres-platform-
// only, so these drive the HTTP handlers against a REAL PostgreSQL database
// via reeltest.OpenStore, gated on POSTGRES_PLATFORM_TEST_DSN (skips without
// it, same convention as the store/service tests).
package reel_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/thg/scraper/internal/store/reel/reeltest"
)

// postJSON drives a POST and returns the status code plus decoded JSON body.
func postJSON(t *testing.T, app *fiber.App, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1) // -1: disable the default 1s test timeout (real Postgres round-trips)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return resp.StatusCode, out
}

// createReel POSTs a draft and returns its id, failing on non-201.
func createReel(t *testing.T, app *fiber.App) int64 {
	t.Helper()
	code, out := postJSON(t, app, "/reels/", `{"title":"Summer promo","brief":"30s"}`)
	if code != 201 {
		t.Fatalf("CreateDraft = %d, want 201 (body %v)", code, out)
	}
	id, ok := out["reel_id"].(float64)
	if !ok || id <= 0 {
		t.Fatalf("CreateDraft body = %v, want reel_id > 0", out)
	}
	return int64(id)
}

func TestReelAPI_Workflow_HappyPath(t *testing.T) {
	s := reeltest.OpenStore(t)
	const org, user int64 = 6001, 1
	reeltest.CleanupOrgs(t, s, org)
	app := testApp(s, true, org, user)

	reelID := createReel(t, app)

	code, out := postJSON(t, app, fmt.Sprintf("/reels/%d/script", reelID), ``)
	if code != 201 {
		t.Fatalf("GenerateScript = %d, want 201 (body %v)", code, out)
	}
	if v, _ := out["version"].(float64); v != 1 {
		t.Fatalf("GenerateScript version = %v, want 1", out["version"])
	}

	if code, _ := postJSON(t, app, fmt.Sprintf("/reels/%d/approve", reelID), ``); code != 200 {
		t.Fatalf("ApproveLatestScript = %d, want 200", code)
	}
	if code, _ := postJSON(t, app, fmt.Sprintf("/reels/%d/render/fake", reelID), ``); code != 200 {
		t.Fatalf("RenderFake = %d, want 200", code)
	}
}

func TestReelAPI_RenderBeforeApproval_409(t *testing.T) {
	s := reeltest.OpenStore(t)
	const org, user int64 = 6002, 1
	reeltest.CleanupOrgs(t, s, org)
	app := testApp(s, true, org, user)

	reelID := createReel(t, app)
	if code, _ := postJSON(t, app, fmt.Sprintf("/reels/%d/script", reelID), ``); code != 201 {
		t.Fatalf("GenerateScript = %d, want 201", code)
	}
	if code, _ := postJSON(t, app, fmt.Sprintf("/reels/%d/render/fake", reelID), ``); code != 409 {
		t.Fatalf("render before approval = %d, want 409", code)
	}
}

func TestReelAPI_ApproveNoScript_404(t *testing.T) {
	s := reeltest.OpenStore(t)
	const org, user int64 = 6003, 1
	reeltest.CleanupOrgs(t, s, org)
	app := testApp(s, true, org, user)

	reelID := createReel(t, app)
	if code, _ := postJSON(t, app, fmt.Sprintf("/reels/%d/approve", reelID), ``); code != 404 {
		t.Fatalf("approve with no script = %d, want 404", code)
	}
}

func TestReelAPI_CrossOrg_CannotAccess_404(t *testing.T) {
	s := reeltest.OpenStore(t)
	const orgA, orgB, user int64 = 6004, 6005, 1
	reeltest.CleanupOrgs(t, s, orgA, orgB)

	appA := testApp(s, true, orgA, user)
	reelID := createReel(t, appA) // owned by orgA

	// orgB must not be able to generate a script on orgA's reel.
	appB := testApp(s, true, orgB, user)
	if code, _ := postJSON(t, appB, fmt.Sprintf("/reels/%d/script", reelID), ``); code != 404 {
		t.Fatalf("cross-org GenerateScript = %d, want 404", code)
	}
}
