package main

import (
	"fmt"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestIsCommentableFacebookPostURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "profile post", url: "https://www.facebook.com/user/posts/123456789", want: true},
		{name: "group permalink", url: "https://www.facebook.com/groups/123/permalink/456/", want: true},
		{name: "group posts path", url: "https://www.facebook.com/groups/123/posts/456/", want: true},
		{name: "story fbid", url: "https://www.facebook.com/story.php?story_fbid=456&id=123", want: true},
		{name: "multi permalinks", url: "https://www.facebook.com/groups/123?multi_permalinks=456", want: true},
		{name: "photo", url: "https://www.facebook.com/photo.php?fbid=456", want: true},
		{name: "fb watch", url: "https://fb.watch/abc123/", want: true},
		{name: "group home is unsafe", url: "https://www.facebook.com/groups/123", want: false},
		{name: "profile home is unsafe", url: "https://www.facebook.com/profile.php?id=123", want: false},
		{name: "facebook home is unsafe", url: "https://www.facebook.com/", want: false},
		{name: "external url", url: "https://example.com/posts/123", want: false},
		{name: "empty", url: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCommentableFacebookPostURL(tt.url); got != tt.want {
				t.Fatalf("isCommentableFacebookPostURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveOutboundTargetURL(t *testing.T) {
	tests := []struct {
		name       string
		lead       models.Lead
		msgType    string
		wantURL    string
		wantReason string
	}{
		{
			name:    "post source comment msg uses SourceURL",
			lead:    models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/groups/123/posts/456/"},
			msgType: "comment",
			wantURL: "https://www.facebook.com/groups/123/posts/456/",
		},
		{
			name:    "comment source comment msg uses SourceURL (parent post)",
			lead:    models.Lead{SourceType: "comment", SourceURL: "https://www.facebook.com/groups/123/posts/456/"},
			msgType: "comment",
			wantURL: "https://www.facebook.com/groups/123/posts/456/",
		},
		{
			name:    "prompt_target source comment msg uses SourceURL",
			lead:    models.Lead{SourceType: "prompt_target", SourceURL: "https://www.facebook.com/user/posts/789"},
			msgType: "comment",
			wantURL: "https://www.facebook.com/user/posts/789",
		},
		{
			name:    "empty source type defaults to SourceURL path",
			lead:    models.Lead{SourceType: "", SourceURL: "https://www.facebook.com/groups/123/posts/456/"},
			msgType: "comment",
			wantURL: "https://www.facebook.com/groups/123/posts/456/",
		},
		{
			name:    "uppercased source type normalises",
			lead:    models.Lead{SourceType: "POST", SourceURL: "https://www.facebook.com/groups/123/posts/456/"},
			msgType: "comment",
			wantURL: "https://www.facebook.com/groups/123/posts/456/",
		},
		{
			name:       "inbox source type rejected for comment",
			lead:       models.Lead{SourceType: "inbox", SourceURL: "https://www.facebook.com/groups/123/posts/456/"},
			msgType:    "comment",
			wantReason: "unrouted_source_type",
		},
		{
			name:       "unknown source type rejected",
			lead:       models.Lead{SourceType: "weird_new_type", SourceURL: "https://www.facebook.com/groups/123/posts/456/"},
			msgType:    "comment",
			wantReason: "unrouted_source_type",
		},
		{
			name:    "photo SourceURL with group+post FBID reconstructs canonical",
			lead:    models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/photo/?fbid=111&set=gm.222", GroupFBID: "123", PostFBID: "456"},
			msgType: "comment",
			wantURL: "https://www.facebook.com/groups/123/posts/456/",
		},
		{
			name:       "photo SourceURL without group_fbid still skipped",
			lead:       models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/photo/?fbid=111", PostFBID: "456"},
			msgType:    "comment",
			wantReason: "missing_post_permalink",
		},
		{
			name:       "photo SourceURL without post_fbid still skipped",
			lead:       models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/photo/?fbid=111", GroupFBID: "123"},
			msgType:    "comment",
			wantReason: "missing_post_permalink",
		},
		{
			name:       "non-commentable URL no FBIDs skipped",
			lead:       models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/groups/123"},
			msgType:    "comment",
			wantReason: "missing_post_permalink",
		},
		{
			name:    "inbox msg uses AuthorURL ignoring SourceURL",
			lead:    models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/groups/123/posts/456/", AuthorURL: "https://www.facebook.com/user.42"},
			msgType: "inbox",
			wantURL: "https://www.facebook.com/user.42",
		},
		{
			name:       "inbox msg missing AuthorURL",
			lead:       models.Lead{SourceType: "inbox"},
			msgType:    "inbox",
			wantReason: "missing_target",
		},
		{
			name:       "non-comment msg with empty SourceURL",
			lead:       models.Lead{SourceType: "post", SourceURL: ""},
			msgType:    "group_post",
			wantReason: "missing_target",
		},
		{
			name:    "non-comment msg passes through without commentable check",
			lead:    models.Lead{SourceType: "post", SourceURL: "https://www.facebook.com/groups/123"},
			msgType: "group_post",
			wantURL: "https://www.facebook.com/groups/123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotReason := resolveOutboundTargetURL(tt.lead, tt.msgType)
			if gotURL != tt.wantURL {
				t.Errorf("url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestResolveProfilePostTarget(t *testing.T) {
	okFetcher := func(profileURL string) accountFetcher {
		return func(accountID, orgID int64) (*models.Account, error) {
			return &models.Account{ID: accountID, OrgID: orgID, FBProfileURL: profileURL}, nil
		}
	}
	errFetcher := func(accountID, orgID int64) (*models.Account, error) {
		return nil, errSentinel
	}
	nilFetcher := func(accountID, orgID int64) (*models.Account, error) {
		return nil, nil
	}

	tests := []struct {
		name         string
		fetch        accountFetcher
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
			name:         "no explicit, no account — refuses (no /me fallback)",
			fetch:        okFetcher("https://www.facebook.com/account.profile"),
			orgID:        7,
			accountID:    0,
			requestedURL: "",
			wantReason:   "no_profile_url_resolved",
		},
		{
			name:         "no explicit, account lookup errors — refuses",
			fetch:        errFetcher,
			orgID:        7,
			accountID:    42,
			requestedURL: "",
			wantReason:   "no_profile_url_resolved",
		},
		{
			name:         "no explicit, account not found — refuses",
			fetch:        nilFetcher,
			orgID:        7,
			accountID:    42,
			requestedURL: "",
			wantReason:   "no_profile_url_resolved",
		},
		{
			name:         "no explicit, account has empty FBProfileURL — refuses (no /me)",
			fetch:        okFetcher(""),
			orgID:        7,
			accountID:    42,
			requestedURL: "",
			wantReason:   "no_profile_url_resolved",
		},
		{
			name:         "no explicit, account has whitespace FBProfileURL — refuses",
			fetch:        okFetcher("   "),
			orgID:        7,
			accountID:    42,
			requestedURL: "",
			wantReason:   "no_profile_url_resolved",
		},
		{
			name:         "nil fetcher with valid account ID — refuses",
			fetch:        nil,
			orgID:        7,
			accountID:    42,
			requestedURL: "",
			wantReason:   "no_profile_url_resolved",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotReason := resolveProfilePostTarget(tt.fetch, tt.orgID, tt.accountID, tt.requestedURL)
			if gotURL != tt.wantURL {
				t.Errorf("url = %q, want %q", gotURL, tt.wantURL)
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

var errSentinel = fmt.Errorf("sentinel fetch error")

func TestCanonicalGroupPostURLFromFBIDs(t *testing.T) {
	tests := []struct {
		name      string
		groupFBID string
		postFBID  string
		want      string
	}{
		{name: "both present", groupFBID: "123", postFBID: "456", want: "https://www.facebook.com/groups/123/posts/456/"},
		{name: "missing group", groupFBID: "", postFBID: "456", want: ""},
		{name: "missing post", groupFBID: "123", postFBID: "", want: ""},
		{name: "both missing", groupFBID: "", postFBID: "", want: ""},
		{name: "whitespace trimmed", groupFBID: "  123  ", postFBID: "  456  ", want: "https://www.facebook.com/groups/123/posts/456/"},
		{name: "whitespace-only treated as empty", groupFBID: "   ", postFBID: "456", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalGroupPostURLFromFBIDs(tt.groupFBID, tt.postFBID); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
