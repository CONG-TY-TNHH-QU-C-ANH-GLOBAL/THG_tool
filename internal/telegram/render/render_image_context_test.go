package render

import (
	"strings"
	"testing"
)

func TestLeadImageContextNoteRemainsSeparateFromReply(t *testing.T) {
	got := Lead(LeadMsg{ContextNote: "Chưa phân tích được ảnh", SuggestedReply: "Bạn gửi giúp mình mã mẫu nhé?"})
	if !strings.Contains(got, "🖼 Ngữ cảnh ảnh: Chưa phân tích được ảnh") || !strings.Contains(got, "💬 Gợi ý trả lời") {
		t.Fatal("image status or copy-ready reply missing")
	}
	if strings.Contains(Lead(LeadMsg{}), "Ngữ cảnh ảnh") {
		t.Fatal("empty image context must not change older notices")
	}
}

func TestLeadAnalysisReviewNoteExplainsMissingDraft(t *testing.T) {
	got := Lead(LeadMsg{ReplyNote: "Bài chưa thể hiện nhu cầu phù hợp"})
	if !strings.Contains(got, "ℹ️ Gợi ý: Bài chưa thể hiện nhu cầu phù hợp") || strings.Contains(got, "💬 Gợi ý trả lời") {
		t.Fatal("noncommercial analysis must have an explanatory note, not a fabricated draft")
	}
	if strings.Contains(Lead(LeadMsg{}), "ℹ️ Gợi ý") {
		t.Fatal("older notices must omit empty analysis notes")
	}
}
