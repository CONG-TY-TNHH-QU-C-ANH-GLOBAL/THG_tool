package connectors

import "testing"

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		v, min string
		want   bool
	}{
		{"0.5.29.878", "0.5.26", true},
		{"0.5.26", "0.5.26", true},
		{"0.5.25", "0.5.26", false},
		{"0.6.0", "0.5.26", true},
		{"0.5", "0.5.26", false}, // missing trailing → 0 < 26
		{"", "0.5.26", false},
	}
	for _, c := range cases {
		if got := versionAtLeast(c.v, c.min); got != c.want {
			t.Fatalf("versionAtLeast(%q,%q)=%v want %v", c.v, c.min, got, c.want)
		}
	}
}

func TestPickReadyConnector(t *testing.T) {
	const acc = int64(50)
	logged := func(id, accountID int64, fb, ver string) AgentToken {
		return AgentToken{ID: id, AssignedAccountID: accountID, Online: true, StreamStatus: "facebook_logged_in", FBUserID: fb, Version: ver}
	}

	// Ready: online + logged in + fb matches + version ok.
	if id, r := PickReadyConnector([]AgentToken{logged(7, acc, "111", "0.5.29")}, acc, "111", MinExtensionVersion); r != ConnReady || id != 7 {
		t.Fatalf("ready: got id=%d reason=%q", id, r)
	}
	// Offline: no connector for the account online.
	if _, r := PickReadyConnector(nil, acc, "111", MinExtensionVersion); r != ConnOffline {
		t.Fatalf("offline empty: %q", r)
	}
	off := logged(7, acc, "111", "0.5.29")
	off.Online = false
	if _, r := PickReadyConnector([]AgentToken{off}, acc, "111", MinExtensionVersion); r != ConnOffline {
		t.Fatalf("offline: %q", r)
	}
	// A connector bound to a DIFFERENT account is ignored → offline.
	if _, r := PickReadyConnector([]AgentToken{logged(7, 999, "111", "0.5.29")}, acc, "111", MinExtensionVersion); r != ConnOffline {
		t.Fatalf("other-account ignored: %q", r)
	}
	// Identity unknown: online+logged in but no fb_user_id.
	if _, r := PickReadyConnector([]AgentToken{logged(7, acc, "", "0.5.29")}, acc, "111", MinExtensionVersion); r != ConnIdentityUnknown {
		t.Fatalf("identity unknown: %q", r)
	}
	// Mismatch: connector logged into a different FB than the account expects.
	if _, r := PickReadyConnector([]AgentToken{logged(7, acc, "222", "0.5.29")}, acc, "111", MinExtensionVersion); r != ConnIdentityMismatch {
		t.Fatalf("mismatch: %q", r)
	}
	// Outdated extension.
	if _, r := PickReadyConnector([]AgentToken{logged(7, acc, "111", "0.5.10")}, acc, "111", MinExtensionVersion); r != ConnExtensionOutdated {
		t.Fatalf("outdated: %q", r)
	}
	// Not logged into Facebook → offline.
	notLogged := logged(7, acc, "111", "0.5.29")
	notLogged.StreamStatus = "idle"
	if _, r := PickReadyConnector([]AgentToken{notLogged}, acc, "111", MinExtensionVersion); r != ConnOffline {
		t.Fatalf("not logged in: %q", r)
	}
}
