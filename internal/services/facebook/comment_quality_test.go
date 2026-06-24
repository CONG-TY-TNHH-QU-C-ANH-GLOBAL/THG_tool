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
	t.Run("empty content is rejected with the sanitize reason", func(t *testing.T) {
		got, skip := ScreenCommentQuality("   ", models.CompanyIdentity{})
		if skip != "comment_quality_empty" {
			t.Fatalf("skip = %q, want comment_quality_empty", skip)
		}
		if got != "" {
			t.Errorf("rejected content must be empty, got %q", got)
		}
	})

	t.Run("placeholder content is rejected with the sanitize reason", func(t *testing.T) {
		got, skip := ScreenCommentQuality("Chào Anonymous participant, inbox em nhé.", models.CompanyIdentity{})
		if skip != "comment_quality_placeholder" {
			t.Fatalf("skip = %q, want comment_quality_placeholder", skip)
		}
		if got != "" {
			t.Errorf("rejected content must be empty, got %q", got)
		}
	})

	t.Run("clean comment with no website to add proceeds", func(t *testing.T) {
		got, skip := ScreenCommentQuality("Sản phẩm đẹp quá, bạn cho mình hỏi thêm thông tin với nhé.", models.CompanyIdentity{})
		if skip != "" {
			t.Fatalf("clean comment must proceed, got skip %q", skip)
		}
		if got == "" {
			t.Errorf("clean comment must return non-empty content")
		}
	})
}
