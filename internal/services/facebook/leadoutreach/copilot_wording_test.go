package leadoutreach

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

// fakeLifecycle is a store-free LeadLifecycleReader stub. The seam (ARCHCM2c Seam 3)
// is what makes noEligibleCommentMessage testable without a real *store.Store.
type fakeLifecycle struct {
	sum models.LifecycleSummary
	err error
}

func (f fakeLifecycle) LeadLifecycleSummary(_ context.Context, _ int64) (models.LifecycleSummary, error) {
	return f.sum, f.err
}

// TestNoEligibleCommentMessage_Enriched pins the lifecycle-enriched reply: the base
// "0 eligible" line plus the inventory/suggestion derived from the summary.
func TestNoEligibleCommentMessage_Enriched(t *testing.T) {
	rec := fakeLifecycle{sum: models.LifecycleSummary{WaitingReply: 2, FollowupDue: 1}}
	got := noEligibleCommentMessage(context.Background(), rec, 7, 9, " skip.")

	if !strings.Contains(got, "quét 9 lead") {
		t.Errorf("missing scanned count: %q", got)
	}
	if !strings.Contains(got, " skip.") {
		t.Errorf("missing skipNote: %q", got)
	}
	if !strings.Contains(got, "đang chờ phản hồi") {
		t.Errorf("missing lifecycle inventory: %q", got)
	}
}

// TestNoEligibleCommentMessage_FallsBackOnError: a read error degrades to the bare
// base line (no suggestion), preserving the original error-tolerant behavior.
func TestNoEligibleCommentMessage_FallsBackOnError(t *testing.T) {
	rec := fakeLifecycle{err: errors.New("boom")}
	got := noEligibleCommentMessage(context.Background(), rec, 7, 4, "")

	if !strings.Contains(got, "quét 4 lead") {
		t.Errorf("missing base line: %q", got)
	}
	if strings.Contains(got, "Gợi ý") {
		t.Errorf("suggestion must be absent on error: %q", got)
	}
}

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
