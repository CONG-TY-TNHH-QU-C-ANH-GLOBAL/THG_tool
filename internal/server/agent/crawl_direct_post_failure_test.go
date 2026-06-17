package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/store/coordination"
)

// directPostFailureUserMessage must map the typed terminal codes to the required
// requester-facing Vietnamese messages, with a safe default for anything else.
func TestDirectPostFailureUserMessage(t *testing.T) {
	cases := map[string]string{
		coordination.DPErrImportGroupMismatch:      "lệch group/context",
		coordination.DPErrImportNoObservedItem:     "chưa quan sát được bài viết mục tiêu",
		coordination.DPErrImportBoilerplateContent: "giao diện Facebook",
		coordination.DPErrImportTargetNotRendered:  "chưa hiển thị được bài viết mục tiêu",
		"some_unknown_code":                        "không xác minh được bài viết mục tiêu",
	}
	for code, want := range cases {
		if msg := directPostFailureUserMessage(code); !strings.Contains(msg, want) {
			t.Errorf("code %q → %q, want substring %q", code, msg, want)
		}
	}
}

// P1.3E: the extension-reported typed crawl error maps to the right terminal workflow code.
func TestDirectPostFailureCodeFromExtensionError(t *testing.T) {
	cases := map[string]string{
		"direct_post_target_not_rendered": coordination.DPErrImportTargetNotRendered,
		"wrong_page: expected X got Y":    coordination.DPErrImportTargetNotRendered,
		"direct_post_boilerplate_content": coordination.DPErrImportBoilerplateContent,
		"direct_post_group_mismatch":      coordination.DPErrImportGroupMismatch,
		"direct_post_post_mismatch":       coordination.DPErrImportPostIDMismatch,
		"some other connector error":      coordination.DPErrImportNoObservedItem,
	}
	for errMsg, want := range cases {
		if got := directPostFailureCodeFromExtensionError(errMsg); got != want {
			t.Errorf("extension error %q → %q, want %q", errMsg, got, want)
		}
	}
}

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
