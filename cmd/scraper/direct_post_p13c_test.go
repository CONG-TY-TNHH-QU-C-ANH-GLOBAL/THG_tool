package main

import (
	"testing"

	"github.com/thg/scraper/internal/store/coordination"
)

// B — explicit direct-post import is PINNED to the action account; never auto-picks another.
func TestDirectPostImportArgs_PinsActionAccount(t *testing.T) {
	pinned := directPostImportArgs(directPostCommentInput{OrgID: 5, AccountID: 49, CanonicalPostURL: "u"})
	if got := argInt64(pinned, "account_id"); got != 49 {
		t.Fatalf("import must pin the action account 49, got account_id=%d", got)
	}
	if got := argInt64(pinned, "max_items"); got != 1 {
		t.Errorf("single-post import must cap max_items=1, got %d", got)
	}
	// No action account → account_id is left unset (caller logs); never substituted.
	noacct := directPostImportArgs(directPostCommentInput{OrgID: 5, AccountID: 0, CanonicalPostURL: "u"})
	if _, ok := noacct["account_id"]; ok {
		t.Errorf("no action account must leave account_id unset, got %v", noacct["account_id"])
	}
}

// notifyDirectPostFailed surfaces a clear, secret-free failed reason for known codes and a
// safe generic for unknown ones (never silent).
func TestNotifyDirectPostFailed(t *testing.T) {
	w := &coordination.DirectPostCommentWorkflow{CanonicalPostURL: "https://fb/x"}
	var got string
	notify := func(m string) { got = m }

	notifyDirectPostFailed(notify, w, coordination.DPErrImportNoObservedItem)
	if got == "" || !contains(got, coordination.DPErrImportNoObservedItem) {
		t.Errorf("known code must produce a message containing the code, got %q", got)
	}
	got = ""
	notifyDirectPostFailed(notify, w, "some_unknown_code")
	if got == "" {
		t.Error("unknown code must still produce a non-silent failure message")
	}
	// nil notify is a safe no-op.
	notifyDirectPostFailed(nil, w, coordination.DPErrIdentityMismatch)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
