package facebook

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/notifications"
	"github.com/thg/scraper/internal/telegram/render"
)

// renderedCrawlNotice runs extension-shaped text through the real suggestion
// runner and Telegram renderer, as notifyCrawlLead does.
func renderedCrawlNotice(t *testing.T, post, author string) (models.LeadSuggestion, string) {
	t.Helper()
	runner := notifications.NewSuggestionRunner(1, 2*time.Second)
	done := make(chan models.LeadSuggestion, 1)
	lookup := func(context.Context, string) (*ResolvedSupplier, error) { return nil, nil }
	if !runner.TryWithFallback(func(ctx context.Context) models.LeadSuggestion {
		return BuildLeadSuggestion(ctx, nil, nil, nil, 1, post, author, nil, lookup)
	}, func(s models.LeadSuggestion) { done <- s }, UnavailableLeadSuggestion(post, author)) {
		t.Fatal("runner refused the lead")
	}
	s := <-done
	return s, render.Lead(render.LeadMsg{Author: author, Excerpt: post, SuggestedReply: s.Reply})
}

func TestSupplierRequestsReachTelegramDraft(t *testing.T) {
	for name, tc := range map[string]struct{ post, wantIn, notIn string }{
		"plural suppliers":     {"Hello, I am currently looking for reliable suppliers for winter products. Interested in cooperation.", "specific product", ""},
		"manufacturer":         {"Looking for a manufacturer for winter products, can you help?", "specific product", ""},
		"anyone know":          {"Does anyone know a reliable supplier for winter products? Thanks", "specific product", ""},
		"supplier needed":      {"Supplier needed for winter products, shipping to UAE please", "specific product", "destination"},
		"Dubai is destination": {"Looking for a reliable supplier/agent shipping to Dubai", "quantity", "destination"},
		// A two-word commenter name containing "Fulfillment" used to hit the warehouse guard.
		"comment with fulfillment": {"LOOKING FOR A RELIABLE SUPPLIER/AGENT SHIPPING TO UAE Xem bản dịch 1 Jerry Fulfillment · We have a warehouse in Dubai, inbox me", "quantity", "destination"},
	} {
		t.Run(name, func(t *testing.T) {
			s, msg := renderedCrawlNotice(t, tc.post, "Nass PK")
			if !strings.HasPrefix(s.Reply, "Hi Nass PK") || !strings.Contains(s.Reply, tc.wantIn) || !strings.Contains(msg, "💬 Gợi ý trả lời") {
				t.Fatalf("supplier request lost its English draft: %+v\n%s", s, msg)
			}
			if tc.notIn != "" && strings.Contains(s.Reply, tc.notIn) {
				t.Fatalf("draft asks for %q although the post states it: %q", tc.notIn, s.Reply)
			}
			if strings.ContainsAny(s.Reply, "¥$") || strings.Contains(s.Reply, "http") {
				t.Fatalf("generic request must not carry an invented offer: %q", s.Reply)
			}
		})
	}
}

func TestVietnameseSupplierRequestGetsVietnameseDraft(t *testing.T) {
	for _, post := range []string{"Mình đang tìm nhà cung cấp uy tín ship hàng đi UAE", "Minh dang tim nha cung cap uy tin, ai co inbox"} {
		s, msg := renderedCrawlNotice(t, post, "Lan")
		if !strings.HasPrefix(s.Reply, "Lan, bên mình") || !strings.Contains(s.Reply, "sản phẩm cụ thể") || !strings.Contains(msg, "💬 Gợi ý trả lời") {
			t.Fatalf("Vietnamese supplier request lost its draft: %q", s.Reply)
		}
	}
}

func TestCommentsDoNotTurnAdviceIntoSalesLead(t *testing.T) {
	post := "Heads up FBA sellers: most reimbursement claims now have about a 60-day window. Xem bản dịch 4 Sam Lee · Need a sourcing agent? DM me"
	if s, msg := renderedCrawlNotice(t, post, "Anthony Gasser"); s.Reply != "" || strings.Contains(msg, "Gợi ý trả lời") {
		t.Fatalf("a seller's comment must not create a sales draft for an advice post: %+v", s)
	}
}

// Production shape (names replaced): an English post shown in the Vietnamese
// Facebook UI, followed by two comments and their action labels.
func TestProductionCrawlShapeKeepsOnlyPostBody(t *testing.T) {
	post := "Facebook Poster Name 22 phút trước · LOOKING FOR A RELIABLE SUPPLIER/AGENT SHIPPING TO UAE Xem bản dịch 3 1 Abcfulfillment · 9 phút · Theo dõi May I ask what product you need? Trả lời Xem bản dịch Chia sẻ Other Seller · 1 phút here Trả lời Xem bản dịch Chia sẻ Viết câu trả lời... Facebook"
	if got := leadPostBody(post); got != "LOOKING FOR A RELIABLE SUPPLIER/AGENT SHIPPING TO UAE" {
		t.Fatalf("post body = %q", got)
	}
	s, msg := renderedCrawlNotice(t, post, "Poster Name")
	if !strings.Contains(s.Reply, "specific product") || strings.Contains(s.Reply, "destination") || !strings.Contains(msg, "💬 Gợi ý trả lời") {
		t.Fatalf("UAE supplier request must ask only for product and quantity: %q", s.Reply)
	}
}

// Vietnamese posts have no "Xem bản dịch" label; the comment header is the boundary.
func TestVietnameseCommentDoesNotCreateSourcingIntent(t *testing.T) {
	// Before the comment boundary, "nhập hàng 1688" in the seller's comment produced a sourcing draft.
	post := "Facebook Poster Name 1 giờ trước · Mọi người cho mình xin kinh nghiệm bán Amazon cho người mới với ạ 2 1 Nguồn Hàng Express · 5 phút · Theo dõi Bên em nhận nhập hàng 1688 giá tốt, ib em Trả lời Chia sẻ"
	if s, _ := renderedCrawlNotice(t, post, "Poster Name"); s.Reply != "" {
		t.Fatalf("a seller's comment must not make an advice question a sourcing lead: %+v", s)
	}
	body := "Cần nhập 300 cái áo hoodie · 20 ngày giao hàng, ai nhận inbox"
	if got := leadPostBody(body); got != body {
		t.Fatalf("a post sentence with a delivery time is not a comment header: %q", got)
	}
}

func TestSupplierPluralForConcreteProductKeepsQuery(t *testing.T) {
	if got := supplierQuery("Looking for reliable suppliers for winter jackets shipped to UAE"); got != "winter jackets" {
		t.Fatalf("named product must stay searchable, got %q", got)
	}
	if got := supplierQuery("We are looking for a vendor who can ship to UAE"); got != "" {
		t.Fatalf("a role word is not a product to search, got %q", got)
	}
}
