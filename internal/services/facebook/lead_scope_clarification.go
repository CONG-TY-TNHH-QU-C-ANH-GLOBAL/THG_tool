package facebook

import (
	"context"

	"github.com/thg/scraper/internal/ai/leadcontext"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// Scope uncertainty is an operator review concern, not a mandatory customer
// questionnaire. Try the existing bounded lookup for a supported primary item.
// Never quote freight while the purchase target still needs review.
func scopeSourcingSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, orgID int64, post, author string, need leadcontext.Context, sourcing SupplierLookupFunc) LeadSuggestion {
	if early, ok := earlyLeadSuggestion(post, author); ok && early.Reply == "" {
		return early // Preserve explicit non-product exclusions, e.g. VA recruitment.
	}
	out := LeadSuggestion{}
	if primary, ok := scopeSourcingNeed(post, need); ok {
		out = buildGroundedLeadSuggestion(ctx, builder, orgID, leadContextSearchText(post, primary), author, nil, sourcing)
	}
	out.AnalysisStatus, out.ReplyStatus = "scope_review", "grounded_template"
	out.ReplyNote = "Đối tượng khách muốn tìm nguồn còn cần đối chiếu; phương án tìm được chỉ để tham khảo, chưa xác nhận đúng mẫu."
	if !out.Supplier.HasOffer() && out.ProductURL == "" {
		out.Reply, out.ReplyStatus = proactiveSourcingReply(post), "proactive_no_offer"
	}
	return out
}

func scopeSourcingNeed(post string, need leadcontext.Context) (leadcontext.Context, bool) {
	if link, _ := leadMarketplaceURL(post); link != "" {
		return need, true // An exact listing retains precedence over model hints.
	}
	a, b := need.Scope.Alternatives[0], need.Scope.Alternatives[1]
	aInText, bInText := containsLeadPhrase(post, a), containsLeadPhrase(post, b)
	if aInText && bInText {
		return need, false // Do not search a combined "yarn or basket" query.
	}
	if supplierQuery(post) != "" {
		return need, true
	}
	if aInText != bInText {
		need.Product = a
		if bInText {
			need.Product = b
		}
		return need, true
	}
	// With no named text category, a validated visible category may be searched
	// as a reference. Two explicit undecided text options must not be picked at random.
	visualPrimary := !aInText && need.Product != "" && need.Product == need.Visual.Product &&
		(need.Product == a || need.Product == b) && len(need.Visual.Observations) > 0
	return need, visualPrimary
}
