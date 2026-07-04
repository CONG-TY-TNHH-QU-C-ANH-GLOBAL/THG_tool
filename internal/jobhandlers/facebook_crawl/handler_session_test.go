package facebookcrawl

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/thg/scraper/internal/jobs"
	"github.com/thg/scraper/internal/livesession"
	"github.com/thg/scraper/internal/scoring"
	"github.com/thg/scraper/internal/session"
	"github.com/thg/scraper/internal/store"
)

// PR31D session-acquire safety net: characterize the handler's "allocator wired
// but no idle browser session" branch WITHOUT Docker/Chrome. The session
// constructors do no browser I/O at build time — Docker happens in
// LiveSessionFactory.Wrap, which is reached only on Acquire SUCCESS. So a REAL
// Allocator + factory over an empty browser_sessions table drives the
// acquire-FAILURE path deterministically. E4 finding: the fakeable seam already
// exists via the real constructors; no production seam was introduced.

// TestHandle_NoIdleSessionFailsLoud pins the no-session contract: when the worker
// has a session allocator + live-session factory wired but no idle Facebook
// browser session is available, the crawl fails loud (error, no result) wrapping
// session.ErrNoIdleSession — it never silently produces leads and never reaches
// the Docker Wrap path. accountID 0 (any) and non-zero (sticky) take different
// message branches; both must wrap ErrNoIdleSession.
func TestHandle_NoIdleSessionFailsLoud(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "sess.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// store.New already bootstrapped the (empty) browser_sessions table via app.Migrate
	sqlDB := db.DB()
	sm := session.NewStateMachine(sqlDB)
	alloc := session.NewAllocator(sqlDB, sm)
	factory := livesession.NewLiveSessionFactory(sqlDB, alloc)

	h := New(nil, scoring.New(scoring.DefaultConfig()), nil, nil)
	h.SetAllocator(alloc, factory) // wired, but the sessions table is empty

	cases := []struct {
		name      string
		accountID int64
	}{
		{"any session (account 0)", 0},
		{"sticky account", 77},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(jobs.Task{
				TaskID:    "t1",
				OrgID:     1,
				AccountID: tc.accountID,
				Intent:    "scrape_group",
				CrawlPlan: jobs.CrawlPlan{Sources: []jobs.Source{{Type: "group", URL: "https://facebook.com/groups/123"}}, MaxItems: 50},
			})
			if err != nil {
				t.Fatalf("marshal task: %v", err)
			}
			got, err := h.Handle(context.Background(), &jobs.Job{ID: 1, TaskID: "t1", Intent: "scrape_group", Payload: string(payload)})
			if err == nil {
				t.Fatalf("no idle session must fail loud; got result=%q nil err", got)
			}
			if !errors.Is(err, session.ErrNoIdleSession) {
				t.Fatalf("error must wrap session.ErrNoIdleSession; got %v", err)
			}
			if got != "" {
				t.Fatalf("failure path must emit no result; got %q", got)
			}
		})
	}
}
