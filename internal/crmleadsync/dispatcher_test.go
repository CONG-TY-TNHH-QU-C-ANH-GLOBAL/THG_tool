package crmleadsync

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/thg/scraper/internal/leadingest"
	_ "modernc.org/sqlite"
)

func outboxDB(t *testing.T, tables ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, table := range tables {
		if _, err := db.Exec(`CREATE TABLE ` + table + ` (
    id INTEGER PRIMARY KEY AUTOINCREMENT, event_key TEXT NOT NULL UNIQUE, payload_json TEXT NOT NULL,
    state TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, available_at INTEGER NOT NULL DEFAULT 0,
    locked_until INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME, updated_at DATETIME)`); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func stateOf(t *testing.T, db *sql.DB, table string) (string, int) {
	t.Helper()
	var state string
	var attempts int
	if err := db.QueryRow(`SELECT state, attempts FROM ` + table).Scan(&state, &attempts); err != nil {
		t.Fatal(err)
	}
	return state, attempts
}

// NewHub đang lỗi KHÔNG được làm chậm hay chặn CRM: hai đích, hai outbox, hai lịch thử lại.
func TestNewHubFailureDoesNotHoldBackCRM(t *testing.T) {
	db := outboxDB(t, CRMOutboxTable, NewHubOutboxTable)
	var crmCalls, hubCalls atomic.Int32
	crm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		crmCalls.Add(1)
		if r.Header.Get("x-thg-integration-key") != "crm-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer crm.Close()
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hubCalls.Add(1)
		if r.Header.Get("x-thg-integration-key") != "hub-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer hub.Close()

	d := NewDispatcher(NewStore(db), crm.URL, "crm-key")
	if !d.AddTarget("NewHub", NewStoreFor(db, NewHubOutboxTable), hub.URL, "hub-key") {
		t.Fatal("NewHub target should be added when URL and key are set")
	}
	if got := d.Targets(); len(got) != 2 || got[0] != "CRM" || got[1] != "NewHub" {
		t.Fatalf("targets = %v", got)
	}
	event := leadingest.LeadEvent{OrgID: 7, AuthorName: "A", PostURL: "https://facebook.com/groups/1/posts/2", Excerpt: "Need POD", Category: "hot"}
	if err := d.Enqueue(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	d.Dispatch(context.Background())

	if state, _ := stateOf(t, db, CRMOutboxTable); state != "succeeded" {
		t.Fatalf("CRM state = %s, want succeeded", state)
	}
	if state, attempts := stateOf(t, db, NewHubOutboxTable); state != "pending" || attempts != 1 {
		t.Fatalf("NewHub state = %s attempts = %d, want pending/1 (retried later)", state, attempts)
	}
	if crmCalls.Load() != 1 || hubCalls.Load() != 1 {
		t.Fatalf("calls crm=%d hub=%d", crmCalls.Load(), hubCalls.Load())
	}
}

// Không cấu hình NewHub ⇒ không thêm đích, CRM chạy y như cũ.
func TestNewHubUnconfiguredLeavesCRMAlone(t *testing.T) {
	db := outboxDB(t, CRMOutboxTable)
	d := NewDispatcher(NewStore(db), "https://crm.example/leads", "crm-key")
	if d.AddTarget("NewHub", NewStoreFor(db, NewHubOutboxTable), "", "hub-key") || d.AddTarget("NewHub", NewStoreFor(db, NewHubOutboxTable), "https://hub", "") {
		t.Fatal("NewHub must stay off without both URL and key")
	}
	if got := d.Targets(); len(got) != 1 || got[0] != "CRM" {
		t.Fatalf("targets = %v", got)
	}
}

func TestStoreRejectsUnknownTable(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unknown table")
		}
	}()
	NewStoreFor(nil, "users; DROP TABLE x")
}
