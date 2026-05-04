package server

import (
	"testing"

	"github.com/thg/scraper/internal/store"
)

func TestLocalSessionStatusFromStreamDoesNotTreatTransientChromeAsHardError(t *testing.T) {
	if got := store.LocalSessionStatusFromStream("chrome_not_connected"); got != store.SessionStarting {
		t.Fatalf("chrome_not_connected mapped to %q, want %q", got, store.SessionStarting)
	}
}

func TestLocalSessionStatusFromStreamKeepsHardAndReadyStates(t *testing.T) {
	cases := map[string]store.LocalSessionStatus{
		"facebook_logged_in":      store.SessionReady,
		"facebook_login_required": store.SessionLoginReq,
		"facebook_human_required": store.SessionHumanReq,
		"unknown":                 store.SessionActive,
	}
	for input, want := range cases {
		if got := store.LocalSessionStatusFromStream(input); got != want {
			t.Fatalf("%s mapped to %q, want %q", input, got, want)
		}
	}
}
