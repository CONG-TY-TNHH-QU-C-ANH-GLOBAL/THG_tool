package ai

import (
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeComment_DedupesDoubledGeneration(t *testing.T) {
	// The EXACT failure class the founder reported: the whole comment emitted
	// twice with no separator at the seam ("nhé.Bên em ...").
	one := "Bên em có hỗ trợ sourcing từ Việt Nam và Trung Quốc cho các sản phẩm POD, nếu bạn cần thì inbox em nhé."
	doubled := one + one // "...nhé.Bên em..."
	got, ok, reason := SanitizeComment(doubled)
	if !ok {
		t.Fatalf("doubled comment should be salvageable after dedupe, got reason=%q", reason)
	}
	if got != one {
		t.Fatalf("dedupe did not collapse the doubled comment:\n got: %q\nwant: %q", got, one)
	}
}

func TestSanitizeComment_DedupeVariants(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"space seam", "Inbox em nhé. Inbox em nhé.", "Inbox em nhé."},
		{"newline seam", "Inbox em nhé.\nInbox em nhé.", "Inbox em nhé."},
		{"repeated CTA among real text", "Bên em hỗ trợ sourcing VN/TQ. Inbox em nhé. Inbox em nhé.", "Bên em hỗ trợ sourcing VN/TQ. Inbox em nhé."},
		{"no duplication preserved", "Bên em hỗ trợ sourcing VN/TQ. Inbox em nhé.", "Bên em hỗ trợ sourcing VN/TQ. Inbox em nhé."},
	}
	for _, c := range cases {
		got, ok, _ := SanitizeComment(c.in)
		if !ok || got != c.want {
			t.Fatalf("%s: got %q (ok=%v), want %q", c.name, got, ok, c.want)
		}
	}
}

func TestSanitizeComment_Rejects(t *testing.T) {
	if _, ok, reason := SanitizeComment("   "); ok || reason != "comment_quality_invalid" {
		t.Fatalf("empty: ok=%v reason=%q", ok, reason)
	}
	if _, ok, reason := SanitizeComment("Chào Anonymous participant, inbox em nhé."); ok || reason != "comment_quality_invalid" {
		t.Fatalf("anonymous salutation: ok=%v reason=%q", ok, reason)
	}
	// DISTINCT sentences (dedupe must not collapse them) that together exceed the
	// length cap.
	var sb strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, "Câu số %d là một câu khác nhau để vượt giới hạn độ dài. ", i)
	}
	if _, ok, reason := SanitizeComment(sb.String()); ok || reason != "comment_quality_invalid" {
		t.Fatalf("over-length: ok=%v reason=%q", ok, reason)
	}
}
