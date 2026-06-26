package main

import "testing"

// Characterization tests pinning the two non-trivial pure parsers moved into
// action_config.go (ARCHCM1 split). Behavior must be unchanged by the move.

func TestMaxItemsFromPrompt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"crawl this group", 0},
		{"lấy 30 bài mới nhất", 30},  // "30 bai" form
		{"cào 12 post", 12},          // "cao N" form
		{"crawl 999 posts", 200},     // clamped to 200
		{"lấy 0 bài", 0},             // non-positive ignored
	}
	for _, c := range cases {
		if got := maxItemsFromPrompt(c.in); got != c.want {
			t.Errorf("maxItemsFromPrompt(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPromptKeywordFallback(t *testing.T) {
	// URL stripped, stopwords/short tokens dropped, deduped.
	got := promptKeywordFallback("Cào https://x.test áo thun mèo áo thun")
	if got == "" {
		t.Fatal("expected keywords, got empty")
	}
	// "cào" is a stopword and the URL is removed; "áo" is dropped as a
	// 2-rune token (the <3-rune filter); "thun"/"mèo" survive and dedupe.
	if want := "thun, mèo"; got != want {
		t.Errorf("promptKeywordFallback = %q, want %q", got, want)
	}
	if promptKeywordFallback("   ") != "" {
		t.Error("blank prompt should yield empty string")
	}
}

func TestURLClassifiers(t *testing.T) {
	if got := sourceTypeFromURL("https://facebook.com/groups/123"); got != "facebook_group" {
		t.Errorf("group url = %q", got)
	}
	if got := sourceTypeFromURL("https://facebook.com/groups/123/posts/456"); got != "facebook_post" {
		t.Errorf("post url = %q", got)
	}
	if got := sourceTypeFromURL("https://example.com/x"); got != "web_url" {
		t.Errorf("web url = %q", got)
	}
}
