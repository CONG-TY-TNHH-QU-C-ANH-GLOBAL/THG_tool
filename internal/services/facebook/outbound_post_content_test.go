package facebook

import (
	"context"
	"testing"
)

// With no generator available, ResolveFacebookPostContent uses the first non-empty
// of content → description → title (the deterministic fallback chain).
func TestResolveFacebookPostContent_FirstNonEmptyFallback(t *testing.T) {
	ctx := context.Background()

	// content wins when present.
	got, err := ResolveFacebookPostContent(ctx, nil, FacebookPostContentInput{
		Content: "explicit body", Description: "desc", Title: "title",
	})
	if err != nil || got != "explicit body" {
		t.Fatalf("content must win: got %q err %v", got, err)
	}

	// falls back to description, then title.
	got, err = ResolveFacebookPostContent(ctx, nil, FacebookPostContentInput{Description: "desc", Title: "title"})
	if err != nil || got != "desc" {
		t.Fatalf("description fallback: got %q err %v", got, err)
	}
	got, err = ResolveFacebookPostContent(ctx, nil, FacebookPostContentInput{Title: "title"})
	if err != nil || got != "title" {
		t.Fatalf("title fallback: got %q err %v", got, err)
	}
}

// No content of any kind → a typed error (message preserved). nil generator means
// no AI path is attempted.
func TestResolveFacebookPostContent_RequiresContent(t *testing.T) {
	_, err := ResolveFacebookPostContent(context.Background(), nil, FacebookPostContentInput{})
	if err == nil || err.Error() != "Facebook post content is required" {
		t.Fatalf("empty input must error, got %v", err)
	}
}
