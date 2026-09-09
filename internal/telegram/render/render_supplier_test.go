package render

import (
	"strings"
	"testing"
)

func TestLead_RendersSupplierBlock(t *testing.T) {
	out := Lead(LeadMsg{
		Workspace: "THG Fulfill", Author: "Minh", Excerpt: "Cần tìm xưởng áo hoodie",
		SuggestedReply:   "Hi Minh, ...",
		SupplierPlatform: "1688", SupplierName: "Áo hoodie nỉ bông",
		SupplierURL:     "https://detail.1688.com/offer/1.html",
		SupplierSummary: "¥28.9 · 0.292 kg · MOQ 1 件 · 广东省广州市",
	})
	for _, want := range []string{"🏭 Nguồn hàng 1688: Áo hoodie nỉ bông", "¥28.9 · 0.292 kg", "https://detail.1688.com/offer/1.html"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLead_OmitsSupplierLinesWhenUnmatched(t *testing.T) {
	out := Lead(LeadMsg{Workspace: "THG Fulfill", Author: "Minh", Excerpt: "x"})
	if strings.Contains(out, "Nguồn hàng") {
		t.Errorf("no supplier match must render no supplier lines:\n%s", out)
	}
}

func TestLead_SupplierLabelWithoutPlatformHasNoTrailingSpace(t *testing.T) {
	out := Lead(LeadMsg{Author: "Minh", Excerpt: "x", SupplierName: "Áo hoodie"})
	if !strings.Contains(out, "🏭 Nguồn hàng: Áo hoodie") {
		t.Errorf("unknown marketplace must still render cleanly:\n%s", out)
	}
}
