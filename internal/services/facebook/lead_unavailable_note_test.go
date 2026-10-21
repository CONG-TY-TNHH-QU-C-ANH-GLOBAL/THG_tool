package facebook

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/notifications"
	"github.com/thg/scraper/internal/telegram/render"
)

func TestUnavailableDiscussionStillExplainsMissingTelegramDraft(t *testing.T) {
	post := "Do you use private label, wholesale or arbitrage, and what are the trade-offs?"
	out := notifications.ExplainUnavailableSuggestion(UnavailableLeadSuggestion(post, "Moderator"))
	if out.Reply != "" || out.ReplyNote == "" || out.AnalysisStatus != "" || out.ReplyStatus != "processing_unavailable" {
		t.Fatalf("busy discussion must explain unprocessed analysis, not claim a verdict: %+v", out)
	}
	message := render.Lead(render.LeadMsg{SuggestedReply: out.Reply, ReplyNote: out.ReplyNote})
	if !strings.Contains(message, "ℹ️ Gợi ý") || strings.Contains(message, "💬 Gợi ý trả lời") {
		t.Fatalf("operator cannot distinguish missing analysis from an intentional verdict: %s", message)
	}
}
