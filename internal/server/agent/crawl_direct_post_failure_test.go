package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// TestDirectPostFailureUserMessage and TestDirectPostFailureCodeFromExtensionError moved to
// crawl_direct_post_outcome_test.go alongside the helpers they prove.

func newDPFailureStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "dpfail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedDPWorkflow(t *testing.T, db *store.Store, org, userID, accountID int64) *coordination.DirectPostCommentWorkflow {
	t.Helper()
	wf, err := db.Coordination().CreateOrGetDirectPostCommentWorkflow(context.Background(), coordination.DirectPostWorkflowInput{
		OrgID: org, RequestedByUserID: userID, UserRole: "sales", AccountID: accountID,
		CanonicalPostURL: "https://www.facebook.com/groups/1312868109620530/permalink/2047168032857197/",
		PostFBID:         "2047168032857197", GroupRef: "1312868109620530", Prompt: "comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	return wf
}

// UX surfacing: a terminal direct-post failure records the typed Vietnamese reason in the
// REQUESTER's private Copilot history (Source="status"), keyed by RequestedByUserID.
func TestFailDirectPostImport_SurfacesToRequester(t *testing.T) {
	db := newDPFailureStore(t)
	const org, userID, accountID int64 = 5, 123, 50
	wf := seedDPWorkflow(t, db, org, userID, accountID)
	h := &Handler{db: db}

	h.failDirectPostImport(context.Background(), org, wf, coordination.DPErrImportGroupMismatch, "internal")

	logs, err := db.Prompts().GetPromptHistoryForOrg(org, userID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, l := range logs {
		if l.ActionTaken == "direct_post_failed" {
			found = true
			if l.Success {
				t.Errorf("failure log must have success=false")
			}
			if !strings.Contains(l.AIResponse, "lệch group/context") {
				t.Errorf("group-mismatch must surface the group message, got %q", l.AIResponse)
			}
			if l.ActionArgs != coordination.DPErrImportGroupMismatch {
				t.Errorf("log must carry the typed code, got %q", l.ActionArgs)
			}
		}
	}
	if !found {
		t.Fatalf("terminal direct-post failure must create a requester Copilot log, got %d logs", len(logs))
	}
}

// CAS gate: a second failDirectPostImport on an already-terminal workflow must NOT double-surface
// (only the transition that wins records a message).
func TestFailDirectPostImport_NoDoubleSurface(t *testing.T) {
	db := newDPFailureStore(t)
	const org, userID, accountID int64 = 5, 123, 50
	wf := seedDPWorkflow(t, db, org, userID, accountID)
	h := &Handler{db: db}

	h.failDirectPostImport(context.Background(), org, wf, coordination.DPErrImportNoObservedItem, "internal")
	h.failDirectPostImport(context.Background(), org, wf, coordination.DPErrImportGroupMismatch, "internal") // CAS no-op

	logs, _ := db.Prompts().GetPromptHistoryForOrg(org, userID, 10)
	n := 0
	for _, l := range logs {
		if l.ActionTaken == "direct_post_failed" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("only the winning CAS transition may surface a message, got %d", n)
	}
}
