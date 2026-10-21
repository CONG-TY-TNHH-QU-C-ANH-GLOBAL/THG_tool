package facebook

import (
	"context"
	"testing"

	"github.com/thg/scraper/internal/ai/leadcontext"
)

func TestLeadPostBodyDropsRoleHeaders(t *testing.T) {
	const body = "Need a supplier for cotton bags"
	for _, header := range []string{
		"Facebook Sample Author Quản trị viên · 1 ngày trước · ",
		"Facebook Sample Author Người kiểm duyệt · 2 giờ trước · ",
		"Facebook Sample Author Admin · 3 days ago · ",
		"Facebook Sample Author Moderator · 4 hrs ago · ",
		"Facebook Sample Author · Admin · 3 days ago · ",
		"Facebook Sample Author 23 phút trước · ",
	} {
		t.Run(header, func(t *testing.T) {
			if got := leadPostBody(header + body + " Xem bản dịch 3 Sample Comment"); got != body {
				t.Fatalf("metadata contaminated post: %q", got)
			}
		})
	}
}

func TestAnalyzedMixedBusinessAndBuyerNeedIsNotKeywordBlocked(t *testing.T) {
	post := "We offer handmade products and need a shipping agent from China to the US."
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.Shipping, "", post), "", &calls)
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Sample", nil, nil, nil)
	if calls != 1 || got.AnalysisStatus != leadcontext.Shipping || got.Reply == "" {
		t.Fatalf("buyer need bypassed: calls=%d analysis=%s reply=%s", calls, got.AnalysisStatus, got.ReplyStatus)
	}
}

func TestAnalyzedAdvertisementStillHasExplanationWithoutPitch(t *testing.T) {
	post := "We provide worldwide shipping. Contact us for our services."
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.NonCommercial, "", post), "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Fatal("an advertisement must not search marketplaces")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Sample", nil, nil, lookup)
	if calls != 1 || got.AnalysisStatus != leadcontext.NonCommercial || got.Reply != "" || got.ReplyNote == "" {
		t.Fatalf("advertisement result unexplained: calls=%d analysis=%s reply=%s", calls, got.AnalysisStatus, got.ReplyStatus)
	}
}

func TestLeadPostBodyPreservesOrdinaryRoleWordsAndTimes(t *testing.T) {
	for _, body := range []string{
		"Need a moderator for our Facebook shop; delivery is 3 days.",
		"Facebook is our channel. Need a supplier for cotton bags.",
		"Need 20 shirts · delivery in 3 days · custom logo.",
	} {
		if got := leadPostBody(body); got != body {
			t.Fatalf("genuine body truncated: %q", got)
		}
	}
}
