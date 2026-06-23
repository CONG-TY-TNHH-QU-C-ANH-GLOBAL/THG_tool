package facebook

import (
	"errors"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Pins the Facebook profile-post target resolution moved out of cmd/scraper (Phase C).
// Behavior is identical to the prior cmd/scraper TestResolveProfilePostTarget: explicit
// profile_url wins; else the account's FBProfileURL; else refuse with no_profile_url_resolved
// (never an implicit /me fallback).
func TestResolveProfilePostTarget(t *testing.T) {
	okFetcher := func(profileURL string) AccountFetcher {
		return func(accountID, orgID int64) (*models.Account, error) {
			return &models.Account{ID: accountID, OrgID: orgID, FBProfileURL: profileURL}, nil
		}
	}
	errSentinel := errors.New("sentinel fetch error")
	errFetcher := func(accountID, orgID int64) (*models.Account, error) {
		return nil, errSentinel
	}
	nilFetcher := func(accountID, orgID int64) (*models.Account, error) {
		return nil, nil
	}

	tests := []struct {
		name         string
		fetch        AccountFetcher
		orgID        int64
		accountID    int64
		requestedURL string
		wantURL      string
		wantReason   string
	}{
		{
			name:         "explicit profile_url wins",
			fetch:        okFetcher("https://www.facebook.com/account.profile"),
			orgID:        7,
			accountID:    42,
			requestedURL: "https://www.facebook.com/explicit",
			wantURL:      "https://www.facebook.com/explicit",
		},
		{
			name:         "explicit profile_url with whitespace trimmed",
			fetch:        nil,
			orgID:        7,
			accountID:    0,
			requestedURL: "  https://www.facebook.com/explicit  ",
			wantURL:      "https://www.facebook.com/explicit",
		},
		{
			name:         "falls back to account FBProfileURL when no explicit",
			fetch:        okFetcher("https://www.facebook.com/account.profile"),
			orgID:        7,
			accountID:    42,
			requestedURL: "",
			wantURL:      "https://www.facebook.com/account.profile",
		},
		{
			name:       "no explicit, no account — refuses (no /me fallback)",
			fetch:      okFetcher("https://www.facebook.com/account.profile"),
			orgID:      7,
			accountID:  0,
			wantReason: "no_profile_url_resolved",
		},
		{
			name:       "no explicit, account lookup errors — refuses",
			fetch:      errFetcher,
			orgID:      7,
			accountID:  42,
			wantReason: "no_profile_url_resolved",
		},
		{
			name:       "no explicit, account not found — refuses",
			fetch:      nilFetcher,
			orgID:      7,
			accountID:  42,
			wantReason: "no_profile_url_resolved",
		},
		{
			name:       "no explicit, account has empty FBProfileURL — refuses (no /me)",
			fetch:      okFetcher(""),
			orgID:      7,
			accountID:  42,
			wantReason: "no_profile_url_resolved",
		},
		{
			name:       "no explicit, account has whitespace FBProfileURL — refuses",
			fetch:      okFetcher("   "),
			orgID:      7,
			accountID:  42,
			wantReason: "no_profile_url_resolved",
		},
		{
			name:       "nil fetcher with valid account ID — refuses",
			fetch:      nil,
			orgID:      7,
			accountID:  42,
			wantReason: "no_profile_url_resolved",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotReason := ResolveProfilePostTarget(tt.fetch, tt.orgID, tt.accountID, tt.requestedURL)
			if gotURL != tt.wantURL {
				t.Errorf("url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}
