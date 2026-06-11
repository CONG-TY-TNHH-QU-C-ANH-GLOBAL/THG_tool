package main

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestJoinVietnamese(t *testing.T) {
	cases := map[string][]string{
		"":          {},
		"a":         {"a"},
		"a và b":    {"a", "b"},
		"a, b và c": {"a", "b", "c"},
	}
	for want, in := range cases {
		if got := joinVietnamese(in); got != want {
			t.Errorf("joinVietnamese(%v) = %q, want %q", in, got, want)
		}
	}
}

// The "nothing eligible" copilot line reports the real inventory and a next step.
func TestLifecycleSuggestion(t *testing.T) {
	// Matches the spec example shape: waiting + archived, no fresh actives.
	sum := models.LifecycleSummary{WaitingReply: 12, Archived: 40}
	got := lifecycleSuggestion(sum)
	if !strings.Contains(got, "12 lead đang chờ phản hồi") || !strings.Contains(got, "40 lead đã lưu trữ") {
		t.Errorf("missing inventory counts: %q", got)
	}
	if !strings.Contains(got, "Có 12 lead đang chờ phản hồi và 40 lead đã lưu trữ.") {
		t.Errorf("inventory phrasing wrong: %q", got)
	}
	// No active leads → suggest crawling more; archived present → suggest viewing archive.
	if !strings.Contains(got, "cào thêm lead mới") || !strings.Contains(got, "xem mục đã lưu trữ") {
		t.Errorf("missing suggestions: %q", got)
	}
}

// follow-up-due leads should prompt the follow-up suggestion.
func TestLifecycleSuggestion_FollowupDue(t *testing.T) {
	got := lifecycleSuggestion(models.LifecycleSummary{Active: 3, FollowupDue: 5})
	if !strings.Contains(got, "bật follow-up cho lead đến hạn") {
		t.Errorf("expected follow-up suggestion: %q", got)
	}
	// Active>0 → do NOT suggest crawling more.
	if strings.Contains(got, "cào thêm lead mới") {
		t.Errorf("should not suggest crawl when actives exist: %q", got)
	}
}
