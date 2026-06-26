package leads

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// TestLeadDedupKey pins the de-dup identity precedence used to merge automation
// leads: trimmed AuthorURL wins, else trimmed SourceURL, else a synthetic id.
func TestLeadDedupKey(t *testing.T) {
	cases := []struct {
		name string
		lead models.Lead
		want string
	}{
		{"author url wins (trimmed)", models.Lead{ID: 5, AuthorURL: "  https://fb/u/1 ", SourceURL: "https://fb/p/9"}, "https://fb/u/1"},
		{"blank author falls back to source", models.Lead{ID: 5, AuthorURL: "   ", SourceURL: " https://fb/p/9 "}, "https://fb/p/9"},
		{"no urls falls back to synthetic id", models.Lead{ID: 5}, "lead:5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := leadDedupKey(tc.lead); got != tc.want {
				t.Fatalf("leadDedupKey = %q, want %q", got, tc.want)
			}
		})
	}
}
