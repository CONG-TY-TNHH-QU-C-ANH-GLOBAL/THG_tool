package facebook

import "github.com/thg/scraper/internal/ai/leadcontext"

// A validated purchase ambiguity asks one focused question before any catalog,
// marketplace or rate lookup. Category names are tentative, not matched offers.
func scopeClarificationSuggestion(post string, need leadcontext.Context) LeadSuggestion {
	a, b := need.Scope.Alternatives[0], need.Scope.Alternatives[1]
	out := LeadSuggestion{
		AnalysisStatus: "needs_scope_confirmation", ReplyStatus: "scope_clarification",
		ReplyNote: "Cần xác nhận đối tượng khách muốn tìm nguồn trước khi chọn sản phẩm.",
	}
	if supplierQueryLanguage(post) == "en" {
		out.Reply = "Are you looking for " + a + " or " + b + "?"
		if leadQuantity(post) == 0 {
			out.Reply += " Please share your planned quantity so we can check sourcing options."
		}
		return out
	}
	out.Reply = "Bạn đang cần tìm nguồn " + a + " hay " + b + " ạ?"
	if leadQuantity(post) == 0 {
		out.Reply += " Bạn cho mình xin số lượng dự kiến để kiểm tra nguồn phù hợp nhé."
	}
	return out
}
