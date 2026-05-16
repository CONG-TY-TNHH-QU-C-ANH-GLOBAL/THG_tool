package runtime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCanonicalSourceURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		postURL    string
		postFBID   string
		groupFBID  string
		want       string
		wantSignal string
	}{
		{
			name:       "clean post permalink wins",
			postURL:    "https://www.facebook.com/groups/123/posts/456/",
			want:       "https://www.facebook.com/groups/123/posts/456/",
			wantSignal: URLRepairAnchorClean,
		},
		{
			name:       "empty url + post_fbid synthesises canonical",
			postURL:    "",
			postFBID:   "456",
			groupFBID:  "123",
			want:       "https://www.facebook.com/groups/123/posts/456/",
			wantSignal: URLRepairSynthFromFBID,
		},
		{
			name:       "empty url + post_fbid no group → global permalink",
			postFBID:   "456",
			want:       "https://www.facebook.com/permalink.php?story_fbid=456",
			wantSignal: URLRepairSynthFromFBID,
		},
		{
			name:       "home feed url with no fbid → empty (drops)",
			postURL:    "https://www.facebook.com/home.php?ref=feed",
			want:       "",
			wantSignal: URLRepairDroppedTransient,
		},
		{
			name:       "bare facebook.com → empty",
			postURL:    "https://www.facebook.com/",
			want:       "",
			wantSignal: URLRepairDroppedTransient,
		},
		{
			name:       "bare facebook.com with only querystring → empty",
			postURL:    "https://www.facebook.com/?__cft__=abc",
			want:       "",
			wantSignal: URLRepairDroppedTransient,
		},
		{
			name:       "drift to home.php but post_fbid present → synthesises",
			postURL:    "https://www.facebook.com/home.php",
			postFBID:   "789",
			groupFBID:  "111",
			want:       "https://www.facebook.com/groups/111/posts/789/",
			wantSignal: URLRepairSynthFromFBID,
		},
		{
			name:       "legitimate group shell (search result) kept as-is",
			postURL:    "https://www.facebook.com/groups/abcdef/",
			want:       "https://www.facebook.com/groups/abcdef/",
			wantSignal: URLRepairAnchorClean,
		},
		{
			name:       "all empty inputs → empty",
			want:       "",
			wantSignal: URLRepairDroppedTransient,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, sig := canonicalSourceURL(c.postURL, c.postFBID, c.groupFBID)
			if got != c.want {
				t.Fatalf("canonicalSourceURL(%q, %q, %q) url = %q; want %q",
					c.postURL, c.postFBID, c.groupFBID, got, c.want)
			}
			if sig != c.wantSignal {
				t.Fatalf("canonicalSourceURL(%q, %q, %q) signal = %q; want %q",
					c.postURL, c.postFBID, c.groupFBID, sig, c.wantSignal)
			}
		})
	}
}

func TestIsTransientFacebookURL(t *testing.T) {
	t.Parallel()
	transient := []string{
		"https://www.facebook.com/home.php",
		"https://www.facebook.com/home.php?ref=feed",
		"https://www.facebook.com/watch",
		"https://www.facebook.com/watch/?v=123",
		"https://www.facebook.com/",
		"https://www.facebook.com",
		"https://www.facebook.com/?__cft__=abc",
		"https://m.facebook.com/",
	}
	for _, u := range transient {
		if !isTransientFacebookURL(u) {
			t.Errorf("expected transient: %q", u)
		}
	}
	stable := []string{
		"https://www.facebook.com/groups/123/",
		"https://www.facebook.com/groups/123/posts/456/",
		"https://www.facebook.com/permalink.php?story_fbid=789",
		"https://www.facebook.com/profile.php?id=42",
	}
	for _, u := range stable {
		if isTransientFacebookURL(u) {
			t.Errorf("expected stable: %q", u)
		}
	}
}

func TestParseRawItems_NewFields(t *testing.T) {
	t.Parallel()
	// Mimics the JS extractor's output: one item with clean anchor, one with
	// lazy-rendered anchor (empty post_url) but data-ft post_fbid recovered.
	rows := []map[string]any{
		{
			"id":         "post1",
			"content":    "Hello",
			"author":     "Alice",
			"author_url": "https://www.facebook.com/alice",
			"post_url":   "https://www.facebook.com/groups/111/posts/222/",
			"post_fbid":  "222",
			"group_fbid": "111",
			"reactions":  3,
			"comments":   1,
			"shares":     0,
		},
		{
			"id":         "post2",
			"content":    "World",
			"author":     "Bob",
			"author_url": "https://www.facebook.com/bob",
			"post_url":   "",
			"post_fbid":  "999",
			"group_fbid": "111",
			"reactions":  0,
			"comments":   0,
			"shares":     0,
		},
		{
			"id":        "drift",
			"content":   "Drifted item, no fbid recoverable",
			"author":    "Carol",
			"post_url":  "https://www.facebook.com/home.php",
			"reactions": 0,
		},
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	items, err := parseRawItems(string(raw))
	if err != nil {
		t.Fatalf("parseRawItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].SourceURL != "https://www.facebook.com/groups/111/posts/222/" {
		t.Errorf("item 0 SourceURL = %q", items[0].SourceURL)
	}
	if items[0].PostFBID != "222" || items[0].GroupFBID != "111" {
		t.Errorf("item 0 fbids = %q / %q", items[0].PostFBID, items[0].GroupFBID)
	}
	if items[1].SourceURL != "https://www.facebook.com/groups/111/posts/999/" {
		t.Errorf("item 1 synthesised SourceURL = %q", items[1].SourceURL)
	}
	if items[2].SourceURL != "" {
		t.Errorf("item 2 SourceURL should be empty (drift, no fbid), got %q", items[2].SourceURL)
	}
	wantRepair := []string{URLRepairAnchorClean, URLRepairSynthFromFBID, URLRepairDroppedTransient}
	for i, want := range wantRepair {
		if items[i].URLRepairPath != want {
			t.Errorf("item %d URLRepairPath = %q; want %q", i, items[i].URLRepairPath, want)
		}
	}
}

func TestParseGroupID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want string
	}{
		{"https://www.facebook.com/groups/123/", "123"},
		{"https://www.facebook.com/groups/123/posts/456/", "123"},
		{"https://www.facebook.com/groups/abc123def/", "abc123def"},
		{"https://www.facebook.com/groups/123?ref=foo", "123"},
		{"https://www.facebook.com/home.php", ""},
		{"https://www.facebook.com/", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := parseGroupID(c.url); got != c.want {
			t.Errorf("parseGroupID(%q) = %q; want %q", c.url, got, c.want)
		}
	}
}

func TestExtractPostsJS_ContainsCleaningHelpers(t *testing.T) {
	t.Parallel()
	// Smoke-test that the JS template includes the new helpers so a refactor
	// that drops them will fail loudly here rather than silently emit dirty URLs.
	js := extractPostsJS(50)
	for _, marker := range []string{
		"cleanURL",
		"extractPostID",
		"postIDFromDataFT",
		"post_fbid",
		"group_fbid",
		"top_level_post_id",
	} {
		if !strings.Contains(js, marker) {
			t.Errorf("extractPostsJS missing %q", marker)
		}
	}
}
