package facebook

import (
	"context"
	"strings"
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

func TestLeadReplyBoundaryBeforeExpandedComments(t *testing.T) {
	const body = "mình cần tìm bên fullfill sản phẩm này ạ"
	for _, ui := range []string{"3 6 1 Xem thêm câu trả lời", "3 1 View more replies", "2 View more comments"} {
		raw := "Facebook Sample 59 phút trước · " + body + " " + ui + " Sample Seller · 56 phút Bên e có mẫu này ạ Trả lời Chia sẻ Shop Print on demand · 33 phút · Theo dõi"
		if got := leadPostBody(raw); got != body {
			t.Errorf("comment boundary did not isolate the author's request: %q", got)
		}
	}
	for _, body := range []string{"Cần in logo lên áo, xem thêm câu trả lời giúp mình nhé", "Need 300 shirts, view more replies in our discussion"} {
		if got := leadPostBody(body); got != body {
			t.Errorf("ordinary author text truncated: %q", got)
		}
	}
	const pod = "Cần fullfill áo cotton in theo logo"
	if got := leadPostBody(pod + " 4 2 Xem thêm câu trả lời Shop · 1 phút Bên em nhận"); got != pod || !wantsPersonalizedPOD(got) {
		t.Error("the author's explicit POD request must survive comment removal")
	}
}

func TestCommentOnlyPODQuoteCannotGroundUnifiedAnalysis(t *testing.T) {
	post := "Facebook Sample 59 phút trước · mình cần tìm bên fullfill sản phẩm này ạ 3 6 1 Xem thêm câu trả lời Seller · 56 phút Bên e có mẫu này ạ Trả lời Chia sẻ Shop Print on demand · 33 phút · Theo dõi"
	calls := 0
	gen := contextTestGenerator(t, testNeed(leadcontext.POD, "", "Print on demand"), "", &calls)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) {
		t.Error("a comment-only product/POD hint must not start sourcing")
		return nil, nil
	}
	got := BuildLeadSuggestionWithContext(context.Background(), nil, gen, nil, 7, post, "Sample", nil, nil, lookup)
	if calls != 1 || got.AnalysisStatus != "unavailable" || got.Reply == "" || strings.Contains(got.Reply, "in theo yêu cầu") || got.SourcingNote != "" {
		t.Fatalf("comment quote accepted as author evidence: %+v, calls=%d", got, calls)
	}
}

func TestFulfillmentCaptionFallbackDoesNotSourceCommenterPOD(t *testing.T) {
	for _, spelling := range []string{"fulfill", "fullfill"} {
		post := "Facebook Sample 59 phút trước · mình cần tìm bên " + spelling + " sản phẩm này ạ 3 6 1 Xem thêm câu trả lời Seller · 56 phút Bên e có mẫu này ạ Trả lời Chia sẻ Shop Print on demand · 33 phút · Theo dõi"
		lookup := func(context.Context, string) (*ResolvedSupplier, error) {
			t.Error("unidentified fulfillment item must not consume marketplace quota")
			return nil, nil
		}
		got := BuildLeadSuggestion(context.Background(), nil, nil, nil, 7, post, "Sample", nil, lookup)
		if got.Reply == "" || !strings.Contains(got.Reply, "loại hàng") || strings.Contains(got.Reply, "in theo yêu cầu") || got.SourcingNote != "" || got.Supplier.HasOffer() {
			t.Errorf("fulfillment fallback contaminated by commenter: %+v", got)
		}
	}
}
