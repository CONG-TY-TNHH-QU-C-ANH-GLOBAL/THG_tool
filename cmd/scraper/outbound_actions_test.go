package main

import "testing"

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
