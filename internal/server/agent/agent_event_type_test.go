package agent

import "testing"

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
