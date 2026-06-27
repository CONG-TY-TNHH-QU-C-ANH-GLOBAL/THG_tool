package main

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// countReadyFacebookAccounts counts only Facebook accounts that are both
// browser-logged-in and active; anything else (wrong platform, logged out,
// inactive) does not count.
func TestCountReadyFacebookAccounts(t *testing.T) {
	accounts := []models.Account{
		{Platform: models.PlatformFacebook, BrowserLoggedIn: true, Status: models.AccountActive},  // ready
		{Platform: models.PlatformFacebook, BrowserLoggedIn: false, Status: models.AccountActive}, // logged out
		{Platform: models.PlatformFacebook, BrowserLoggedIn: true, Status: "inactive"},            // not active
		{Platform: "taobao", BrowserLoggedIn: true, Status: models.AccountActive},                 // wrong platform
		{Platform: models.PlatformFacebook, BrowserLoggedIn: true, Status: models.AccountActive},  // ready
	}
	if got := countReadyFacebookAccounts(accounts); got != 2 {
		t.Fatalf("countReadyFacebookAccounts = %d, want 2", got)
	}
	if got := countReadyFacebookAccounts(nil); got != 0 {
		t.Fatalf("nil accounts → 0, got %d", got)
	}
}
