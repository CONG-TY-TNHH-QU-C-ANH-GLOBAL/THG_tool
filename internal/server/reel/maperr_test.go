// Internal (package reel) test for the error→HTTP mapping, proving no raw
// store error string reaches the client. Pure transport — no store, no
// Postgres needed.
package reel

import (
	"database/sql"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// mapErrBody drives mapErr(err) through a throwaway route and returns the
// status code and raw response body.
func mapErrBody(t *testing.T, err error) (int, string) {
	t.Helper()
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error { return mapErr(c, err) })
	resp, rerr := app.Test(httptest.NewRequest("GET", "/x", nil), -1)
	if rerr != nil {
		t.Fatalf("app.Test: %v", rerr)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode, string(raw)
}

func TestMapErr_NoRowsDoesNotLeak(t *testing.T) {
	code, body := mapErrBody(t, sql.ErrNoRows)
	if code != 404 {
		t.Fatalf("sql.ErrNoRows status = %d, want 404", code)
	}
	if strings.Contains(body, "sql: no rows") {
		t.Fatalf("response leaks raw sql error: %s", body)
	}
	if !strings.Contains(body, errNotFound) {
		t.Fatalf("response = %s, want safe %q message", body, errNotFound)
	}
}

func TestMapErr_UnexpectedReturnsGeneric500(t *testing.T) {
	code, body := mapErrBody(t, errors.New("boom: internal secret detail"))
	if code != 500 {
		t.Fatalf("unexpected err status = %d, want 500", code)
	}
	if strings.Contains(body, "internal secret detail") {
		t.Fatalf("response leaks raw error: %s", body)
	}
	if !strings.Contains(body, errInternal) {
		t.Fatalf("response = %s, want %q", body, errInternal)
	}
}
