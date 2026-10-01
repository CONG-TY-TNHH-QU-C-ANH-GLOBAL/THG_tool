package render

import (
	"strings"
	"testing"
)

func TestLead_RendersSourcingNoteBeforeDraft(t *testing.T) {
	out := Lead(LeadMsg{
		Author: "Lan", Excerpt: "Cần nhập viên bổ khớp cho chó",
		SourcingNote:   "chưa tìm được sản phẩm/nguồn phù hợp — sale cần kiểm tra thủ công",
		SuggestedReply: "Lan, bạn cho mình xin mẫu cụ thể nhé?",
	})
	note := strings.Index(out, "⚠️ Tìm nguồn: chưa tìm được")
	draft := strings.Index(out, "💬 Gợi ý trả lời")
	if note < 0 || draft < 0 || note > draft {
		t.Fatalf("the not-found note must precede the draft:\n%s", out)
	}
}

func TestLead_OmitsSourcingNoteWhenEmpty(t *testing.T) {
	if out := Lead(LeadMsg{Author: "Lan", Excerpt: "x"}); strings.Contains(out, "Tìm nguồn") {
		t.Fatalf("an empty note must render nothing:\n%s", out)
	}
}
