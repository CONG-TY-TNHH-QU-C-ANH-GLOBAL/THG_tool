package server

import (
	"testing"

	"github.com/thg/scraper/internal/store/sessions"
)

func TestLocalSessionStatusFromStreamDoesNotTreatTransientChromeAsHardError(t *testing.T) {
	if got := sessions.LocalSessionStatusFromStream("chrome_not_connected"); got != sessions.SessionStarting {
		t.Fatalf("chrome_not_connected mapped to %q, want %q", got, sessions.SessionStarting)
	}
}

func TestLocalSessionStatusFromStreamKeepsHardAndReadyStates(t *testing.T) {
	cases := map[string]sessions.LocalSessionStatus{
		"facebook_logged_in":      sessions.SessionReady,
		"facebook_login_required": sessions.SessionLoginReq,
		"facebook_human_required": sessions.SessionHumanReq,
		"unknown":                 sessions.SessionActive,
	}
	for input, want := range cases {
		if got := sessions.LocalSessionStatusFromStream(input); got != want {
			t.Fatalf("%s mapped to %q, want %q", input, got, want)
		}
	}
}
