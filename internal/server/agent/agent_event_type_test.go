package agent

import (
	"testing"

	"github.com/thg/scraper/internal/telegram/control"
)

// Pins the action-type → Telegram channel event-key contract used by
// notifyOutboundFinalized. Regression guard: profile_post must map to the same
// post_submitted/post_failed events as group_post — it previously fell through to
// "" so profile-post outcomes produced NO per-org Telegram channel notification
// even though queueProfilePost stores Type="profile_post".
func TestAgentEventType(t *testing.T) {
	cases := []struct {
		name              string
		actionType        string
		verified, success bool
		want              string
	}{
		{"comment verified", "comment", true, true, "comment_verified"},
		{"comment unverified success", "comment", false, true, "comment_unverified"},
		{"comment failed", "comment", false, false, "comment_failed"},
		{"inbox sent", "inbox", false, true, "inbox_sent"},
		{"inbox failed", "inbox", false, false, "inbox_failed"},
		{"group_post submitted", "group_post", false, true, "post_submitted"},
		{"group_post failed", "group_post", false, false, "post_failed"},
		// The fix: profile_post now notifies like group_post (was "").
		{"profile_post submitted", "profile_post", false, true, "post_submitted"},
		{"profile_post failed", "profile_post", false, false, "post_failed"},
		{"post submitted", "post", false, true, "post_submitted"},
		{"post failed", "post", false, false, "post_failed"},
		{"unknown type → no notification", "reaction", false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agentEventType(c.actionType, c.verified, c.success); got != c.want {
				t.Errorf("agentEventType(%q, verified=%v, success=%v) = %q; want %q",
					c.actionType, c.verified, c.success, got, c.want)
			}
		})
	}
}

// TestAgentEventType_EveryQueueableTypeNotifies is the coverage invariant guarding the
// silent-outcome bug class (PR27A's profile_post gap): EVERY outbound type the system
// can actually queue MUST map to a valid, non-empty Telegram event for BOTH success and
// failure — otherwise notifyOutboundFinalized skips NotifyAction and the org operator
// never sees that outcome.
//
// queueable mirrors the only types that reach finalization: draftOutbound validation
// (comment|inbox|group_post|profile_post) ⊇ queueLeadOutreach ("comment"/"inbox") +
// queueFacebookPostTargets (group_post/profile_post). Add a new queueable type here when
// one is introduced — a "" or unknown event key fails this test loudly.
func TestAgentEventType_EveryQueueableTypeNotifies(t *testing.T) {
	queueable := []string{"comment", "inbox", "group_post", "profile_post"}
	for _, typ := range queueable {
		for _, success := range []bool{true, false} {
			ev := agentEventType(typ, false, success)
			if ev == "" {
				t.Errorf("queueable type %q (success=%v) maps to NO event — operator gets no notification", typ, success)
				continue
			}
			if !control.IsValidEventType(ev) {
				t.Errorf("queueable type %q (success=%v) maps to %q which is not a valid Telegram event type", typ, success, ev)
			}
		}
	}
	// The comment verified-success variant is a distinct valid event too.
	if ev := agentEventType("comment", true, true); ev == "" || !control.IsValidEventType(ev) {
		t.Errorf("comment verified maps to invalid event %q", ev)
	}
}
