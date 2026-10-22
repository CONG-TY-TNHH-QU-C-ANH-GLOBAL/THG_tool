package facebook

import "github.com/thg/scraper/internal/ai/leadcontext"

func observedProductWithoutOffer(need leadcontext.Context, imageCount int, out LeadSuggestion) bool {
	return imageCount > 0 && need.Visual.Product != "" && len(need.Visual.Observations) > 0 &&
		(need.Intent == leadcontext.Sourcing || need.Intent == leadcontext.POD) && out.Reply != "" && !out.Supplier.HasOffer() && out.ProductURL == ""
}

// This is a contact draft, not a claim that stock, a quote or a matching source
// exists. Lookup failures and tentative identity stay in the operator notes.
func proactiveSourcingReply(post string) string {
	if supplierQueryLanguage(post) == "en" {
		reply := "I can check sourcing options for your request."
		if containsAnyLeadPhrase(post, []string{"european", "supplier in europe", "eu supplier"}) {
			reply = "I can check China-based alternatives to your European sourcing request."
		}
		if leadQuantity(post) == 0 {
			return reply + " Message me with your planned quantity so we can discuss suitable options."
		}
		return reply + " Message me so we can discuss suitable options."
	}
	reply := "Mình có thể kiểm tra nguồn theo mẫu bạn đăng."
	if leadQuantity(post) == 0 {
		return reply + " Bạn nhắn mình số lượng dự kiến để trao đổi phương án phù hợp nhé."
	}
	return reply + " Bạn nhắn mình để trao đổi phương án phù hợp nhé."
}
