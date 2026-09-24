package render

import (
	"strings"
	"testing"
)

func TestLeadOmitsOperatorSuggestionWhenEmpty(t *testing.T) {
	got := Lead(LeadMsg{Workspace: "Acme", Status: "Sẵn sàng xử lý"})
	for _, absent := range []string{"Gợi ý trả lời", "Sản phẩm", "Cước tham chiếu"} {
		if strings.Contains(got, absent) {
			t.Fatalf("a notice with no enrichment must not show %q:\n%s", absent, got)
		}
	}
}

func TestLeadRendersOperatorSuggestion(t *testing.T) {
	got := Lead(LeadMsg{
		SuggestedReply:  "Bạn có thể xem mẫu phù hợp ở đây.",
		ProductName:     "Áo hoodie",
		ProductURL:      "https://catalog.example/p/hoodie",
		ProductImageURL: "https://cdn.example/p/hoodie.png",
	})
	for _, want := range []string{"Gợi ý trả lời", "Bạn có thể xem", "Áo hoodie", "https://catalog.example/p/hoodie"} {
		if !strings.Contains(got, want) {
			t.Fatalf("suggestion notice missing %q:\n%s", want, got)
		}
	}
}

// The dropship shape: a marketplace product with a real price, a landed cost,
// and the basis that cost came from. This is the notice Sale actually acts on,
// so it is pinned line by line.
func TestLeadRendersDropshipShape(t *testing.T) {
	got := Lead(LeadMsg{
		Author:         "Ngọc Trâm Pet Shop",
		Heat:           "🔥 rất nóng",
		SourceLabel:    "Nhóm Facebook — Dropship US",
		Excerpt:        "Shop mình cần nhập viên bổ khớp cho chó từ 1688 về Mỹ",
		ProductLine:    "Thực phẩm bổ sung cho khớp chó · 20 CNY · MOQ 2包",
		ProductURL:     "https://detail.1688.com/offer/1052769132343.html",
		ShippingLine:   "$14.20 · 6–12 ngày làm việc",
		ShippingBasis:  "Epacket CN→US, 1 kiện 0.4 kg",
		SuggestedReply: "Ngọc Trâm Pet Shop ơi, tổng cước epacket cho 0.4 kg là $14.20.",
		PostURL:        "https://facebook.com/groups/1/posts/2",
		CrmURL:         "https://crm.thgfulfill.com",
	})

	for _, want := range []string{
		"🔔 Lead mới · Ngọc Trâm Pet Shop",
		"🔥 rất nóng · Nhóm Facebook — Dropship US",
		"Shop mình cần nhập viên bổ khớp",
		"🛍 Sản phẩm: Thực phẩm bổ sung cho khớp chó · 20 CNY · MOQ 2包",
		"   https://detail.1688.com/offer/1052769132343.html",
		"🚢 Cước tham chiếu: $14.20 · 6–12 ngày làm việc",
		"   Epacket CN→US, 1 kiện 0.4 kg",
		"💬 Gợi ý trả lời:",
		"🔗 Bài gốc",
		"📊 Mở CRM",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dropship notice missing %q:\n%s", want, got)
		}
	}

	// Who and how-hot come before the post text: Sale triages off those two.
	if strings.Index(got, "🔥 rất nóng") > strings.Index(got, "Shop mình cần nhập") {
		t.Fatalf("heat must appear before the post text:\n%s", got)
	}
	// The cost must never appear without the basis it was computed from.
	if strings.Index(got, "🚢 Cước tham chiếu") > strings.Index(got, "Epacket CN→US") {
		t.Fatalf("basis must follow the cost:\n%s", got)
	}
}

// Without a CRM lookup the notice falls back to the bare catalog name, and must
// not print an empty "Cước tham chiếu" header with nothing under it.
func TestLeadWithoutEnrichmentOmitsShipping(t *testing.T) {
	got := Lead(LeadMsg{Author: "A", Heat: "🌤 ấm", Excerpt: "cần tìm xưởng may", ProductName: "Áo thun"})
	if !strings.Contains(got, "🛍 Sản phẩm: Áo thun") {
		t.Fatalf("catalog name should still render:\n%s", got)
	}
	if strings.Contains(got, "Cước tham chiếu") {
		t.Fatalf("no quote means no shipping line at all:\n%s", got)
	}
}
