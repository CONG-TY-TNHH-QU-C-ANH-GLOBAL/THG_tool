package main

import "github.com/thg/scraper/internal/models"

// countReadyFacebookAccounts counts accounts that could run live Facebook
// automation right now (Facebook platform, browser logged-in, active). Pure —
// the scaffold skills use it to reflect whether a live run would be possible.
func countReadyFacebookAccounts(accounts []models.Account) int {
	ready := 0
	for _, a := range accounts {
		if a.Platform == models.PlatformFacebook && a.BrowserLoggedIn && a.Status == models.AccountActive {
			ready++
		}
	}
	return ready
}
