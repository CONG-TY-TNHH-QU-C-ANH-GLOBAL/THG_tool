package control_test

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/telegram/control"
)

func TestSanitizeExcerpt(t *testing.T) {
	// Repeated channel-spam tokens collapse to nothing usable → "" (caller shows fallback).
	if got := control.SanitizeExcerpt("Facebook Facebook Facebook Facebook Facebook"); got != "" {
		t.Fatalf("spam-only must sanitize to empty, got %q", got)
	}
	// Empty / whitespace → "".
	if got := control.SanitizeExcerpt("   \n\t  "); got != "" {
		t.Fatalf("blank must be empty, got %q", got)
	}
	// Real content survives, with a trailing spam repeat collapsed.
	got := control.SanitizeExcerpt("Đang   tìm\n\nsupplier cho mẫu đèn Facebook Facebook Facebook")
	if !strings.Contains(got, "supplier") || strings.Contains(got, "Facebook Facebook") {
		t.Fatalf("content+spam not cleaned: %q", got)
	}
	if strings.Contains(got, "  ") || strings.Contains(got, "\n") {
		t.Fatalf("whitespace not collapsed: %q", got)
	}
	// Long content is trimmed with an ellipsis.
	long := strings.Repeat("alpha beta gamma delta ", 60) // > 300 runes
	out := control.SanitizeExcerpt(long)
	if len([]rune(out)) > 301 || !strings.HasSuffix(out, "…") {
		t.Fatalf("long excerpt not trimmed: len=%d", len([]rune(out)))
	}
}
