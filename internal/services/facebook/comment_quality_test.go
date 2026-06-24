package facebook

import (
	"testing"

	"github.com/thg/scraper/internal/models"
)

// Pins the orchestration contract of the FB comment-quality gate moved here from
// cmd/scraper (Phase C): a sanitize rejection propagates its reason verbatim with empty
// content; a clean comment proceeds. The underlying primitives (SanitizeComment,
// DetectRepeatedText, ScreenCommentContacts, RepairCommentContacts, EnsureWebsite) keep
// their own tests in internal/ai/comment.
func TestScreenCommentQuality(t *testing.T) {
	// wantSkip == "" means the comment must pass through (skip empty, content non-empty);
	// a non-empty wantSkip means rejection (that exact reason, with empty content).
	cases := []struct{ name, content, wantSkip string }{
		{"empty content rejected with sanitize reason", "   ", "comment_quality_empty"},
		{"placeholder content rejected with sanitize reason", "Chào Anonymous participant, inbox em nhé.", "comment_quality_placeholder"},
		{"clean comment with no website to add proceeds", "Sản phẩm đẹp quá, bạn cho mình hỏi thêm thông tin với nhé.", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, skip := ScreenCommentQuality(c.content, models.CompanyIdentity{})
			if skip != c.wantSkip {
				t.Fatalf("skip = %q, want %q", skip, c.wantSkip)
			}
			if c.wantSkip == "" && got == "" {
				t.Errorf("clean comment must return non-empty content")
			}
			if c.wantSkip != "" && got != "" {
				t.Errorf("rejected content must be empty, got %q", got)
			}
		})
	}
}
