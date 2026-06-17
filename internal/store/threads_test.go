// Domain: threads (see internal/store/DOMAINS.md)
package store

import (
	"context"
	"testing"
	"time"
)

func TestSeedThreadForOrg_CreatesRowWithNullOutboundTimestamp(t *testing.T) {
	s := newSharedStore(t, "threads_seed.db")
	const orgID = int64(7)
	const profileURL = "https://www.facebook.com/profile.42"

	id, err := s.Threads().SeedThreadForOrg(orgID, 0, "facebook", profileURL, "Operator Pham", "")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero thread id from seed")
	}

	got, err := s.Threads().GetThreadByProfileForOrg(orgID, profileURL)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got == nil {
		t.Fatal("expected thread, got nil")
	}
	if got.Status != "initiated" {
		t.Errorf("status = %q, want initiated", got.Status)
	}
	if !got.LastOutboundAt.IsZero() {
		t.Errorf("last_outbound_at = %v, want zero (NULL) to allow first send", got.LastOutboundAt)
	}
	if !got.LastInboundAt.IsZero() {
		t.Errorf("last_inbound_at = %v, want zero", got.LastInboundAt)
	}
}

func TestSeedThreadForOrg_IdempotentPreservesFirstAttribution(t *testing.T) {
	s := newSharedStore(t, "threads_seed_idem.db")
	const orgID = int64(7)
	const profileURL = "https://www.facebook.com/profile.99"

	first, err := s.Threads().SeedThreadForOrg(orgID, 11, "facebook", profileURL, "First Touch", "logistics")
	if err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if first == 0 {
		t.Fatal("expected non-zero id on first seed")
	}

	second, err := s.Threads().SeedThreadForOrg(orgID, 99, "facebook", profileURL, "Second Touch", "tuyen_dung")
	if err != nil {
		t.Fatalf("seed second: %v", err)
	}
	if second != first {
		t.Errorf("re-seed returned id %d, want %d (idempotent)", second, first)
	}

	got, err := s.Threads().GetThreadByProfileForOrg(orgID, profileURL)
	if err != nil {
		t.Fatalf("get thread: %v", err)
	}
	if got.ProfileName != "First Touch" {
		t.Errorf("profile_name = %q, want First Touch (first-touch attribution preserved)", got.ProfileName)
	}
	if got.LeadID != 11 {
		t.Errorf("lead_id = %d, want 11 (first attribution preserved)", got.LeadID)
	}
}

func TestSeedThreadForOrg_ConversationGateAllowsFirstSend(t *testing.T) {
	s := newSharedStore(t, "threads_seed_gate.db")
	const orgID = int64(7)
	const profileURL = "https://www.facebook.com/profile.gate"

	if _, err := s.Threads().SeedThreadForOrg(orgID, 0, "facebook", profileURL, "", ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	decision, err := s.conversationGateForOutbound(context.Background(), orgID, profileURL, profileURL, 15*time.Minute)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if !decision.Allowed {
		t.Errorf("first send blocked: reason=%q (seeded thread with NULL last_outbound_at must allow first send)", decision.Reason)
	}
	if decision.Reason != "ok" {
		t.Errorf("reason = %q, want ok", decision.Reason)
	}
}

// TestGetThreadsByOrg_ListsSeededThread is the regression guard for the
// recurring `500 GET /api/threads`. GetThreadsByOrg selects
// COALESCE(last_inbound_at, last_outbound_at, created_at) — a SQL expression
// with NO column type affinity, which modernc/sqlite returns as a string. The
// handler scanned it straight into a time.Time, so the query errored on every
// poll (deterministic, ~1ms) and the handler returned 500. A freshly seeded
// thread (NULL inbound/outbound, real created_at) is the exact production shape.
func TestGetThreadsByOrg_ListsSeededThread(t *testing.T) {
	s := newSharedStore(t, "threads_list_by_org.db")
	const orgID = int64(7)
	const profileURL = "https://www.facebook.com/profile.threadlist"

	if _, err := s.Threads().SeedThreadForOrg(orgID, 0, "facebook", profileURL, "List View", ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rows, err := s.Threads().GetThreadsByOrg(orgID, 100)
	if err != nil {
		t.Fatalf("GetThreadsByOrg returned error (this is the /api/threads 500): %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(rows))
	}
	if rows[0].ProfileName != "List View" {
		t.Errorf("profile_name = %q, want List View", rows[0].ProfileName)
	}
	// last_at must fall back to created_at (inbound/outbound are NULL on a seed).
	if rows[0].LastAt.IsZero() {
		t.Error("last_at should fall back to created_at, got zero time")
	}
}

// TestGetThreadsByOrg_EmptyReturnsNoRows confirms an org with no threads yields
// an empty result and no error (the handler then returns 200 []).
func TestGetThreadsByOrg_EmptyReturnsNoRows(t *testing.T) {
	s := newSharedStore(t, "threads_list_empty.db")
	rows, err := s.Threads().GetThreadsByOrg(999, 100)
	if err != nil {
		t.Fatalf("GetThreadsByOrg(empty org) errored: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 threads, got %d", len(rows))
	}
}

func TestSeedThreadForOrg_RejectsEmptyInputs(t *testing.T) {
	s := newSharedStore(t, "threads_seed_reject.db")
	tests := []struct {
		name       string
		orgID      int64
		profileURL string
	}{
		{name: "zero org", orgID: 0, profileURL: "https://fb.com/x"},
		{name: "negative org", orgID: -1, profileURL: "https://fb.com/x"},
		{name: "empty profile", orgID: 7, profileURL: ""},
		{name: "whitespace profile", orgID: 7, profileURL: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := s.Threads().SeedThreadForOrg(tt.orgID, 0, "facebook", tt.profileURL, "", "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != 0 {
				t.Errorf("expected no-op (id=0), got id=%d", id)
			}
		})
	}
}
