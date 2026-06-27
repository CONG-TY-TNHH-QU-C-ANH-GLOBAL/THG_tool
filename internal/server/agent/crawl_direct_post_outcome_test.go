package agent

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/directpost"
	"github.com/thg/scraper/internal/store/coordination"
)

// C — granular import error-code mapping for an identity-matched-but-poisoned item.
func TestImportContextMismatchCode(t *testing.T) {
	if got := importContextMismatchCode(directpost.ReasonGroupConflict); got != coordination.DPErrImportGroupMismatch {
		t.Errorf("group conflict → %q, want %q", got, coordination.DPErrImportGroupMismatch)
	}
	if got := importContextMismatchCode(directpost.ReasonContentInvalid); got != coordination.DPErrImportBoilerplateContent {
		t.Errorf("content invalid → %q, want %q", got, coordination.DPErrImportBoilerplateContent)
	}
	if got := importContextMismatchCode("anything_else"); got != coordination.DPErrImportRejectedByGuard {
		t.Errorf("unknown → %q, want %q", got, coordination.DPErrImportRejectedByGuard)
	}
}

// D — a FINISHED import with no valid observed item (and no item-level failure) fails the
// workflow deterministically with no_observed_item, instead of silent retry-forever.
func TestDirectPostImportFailureCode(t *testing.T) {
	// Valid item was created → nothing to do.
	if _, fail := directPostImportFailureCode(true, false); fail {
		t.Error("a valid observed item must not fail the workflow")
	}
	// An item-level guard already failed it → nothing to add.
	if _, fail := directPostImportFailureCode(false, true); fail {
		t.Error("an already-failed workflow must not be re-failed")
	}
	// Neither → deterministic no_observed_item failure.
	code, fail := directPostImportFailureCode(false, false)
	if !fail || code != coordination.DPErrImportNoObservedItem {
		t.Errorf("no valid item + no prior failure → fail with %q, got fail=%v code=%q",
			coordination.DPErrImportNoObservedItem, fail, code)
	}
}

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
