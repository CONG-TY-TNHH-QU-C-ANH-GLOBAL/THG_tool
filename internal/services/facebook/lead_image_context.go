package facebook

import (
	"context"
	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/visualcontext"
	"github.com/thg/scraper/internal/platform/facebook/media"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// BuildLeadSuggestionWithImages perceives text + original post images before
// retrieval. Text-only callers retain BuildLeadSuggestion. No inferred image
// price/weight/model becomes supplier or shipping data.
func BuildLeadSuggestionWithImages(ctx context.Context, builder *knowledgeRuntime.Builder, generator *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, post, author string, urls []string, shipping ShippingQuoteFunc, sourcing SupplierLookupFunc) LeadSuggestion {
	reportedImages := len(urls)
	urls = media.URLs(urls)
	if len(urls) == 0 {
		out := BuildLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, shipping, sourcing)
		if reportedImages > 0 || containsAnyLeadPhrase(leadPostBody(post), []string{"photo", "image", "picture", "this model", "mẫu này", "ảnh này", "như ảnh"}) {
			out.ContextNote = "Chưa nhận được ảnh gốc hợp lệ; gợi ý dựa trên chữ, sale cần đối chiếu ảnh."
		}
		return out
	}
	visual, err := generator.AnalyzeLeadImages(ctx, leadPostBody(post), urls)
	if err != nil {
		out := BuildLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, shipping, sourcing)
		out.ContextNote = "Chưa phân tích được ảnh; gợi ý dựa trên chữ, sale cần đối chiếu ảnh."
		return out
	}
	if visual.Conflict {
		return conflictingImageSuggestion(post, author)
	}
	out := BuildLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, shipping, sourcing, visual)
	out.ContextNote = "Đã phân tích ảnh tham chiếu; chưa xác minh đúng mã hàng hoặc mẫu sàn."
	if visual.Product == "" {
		out.ContextNote = "Ảnh chưa đủ rõ để xác định sản phẩm; ưu tiên nội dung chữ và sale đối chiếu thêm."
	}
	return out
}

func imageAwareLeadText(post string, contexts []visualcontext.Context) string {
	link, _ := leadMarketplaceURL(post)
	if len(contexts) == 0 || contexts[0].Product == "" || contexts[0].Conflict || link != "" || supplierQuery(post) != "" {
		return post
	}
	if serviceInquiryKind(post) != "" || containsAnyLeadPhrase(post, serviceOfferTerms) || !containsAnyLeadPhrase(post, serviceNeedTerms) {
		return post
	}
	// This is a tentative generic search hint. Keep the original post separate
	// for generation, quantity, destination and operator display.
	if supplierQueryLanguage(post) == "en" {
		return safeLeadRetrievalHint(post, "Looking for a supplier for "+contexts[0].Product+".\n"+post)
	}
	return safeLeadRetrievalHint(post, "Cần tìm nguồn "+contexts[0].Product+".\n"+post)
}

func visualContextBlock(contexts []visualcontext.Context) string {
	if len(contexts) == 0 {
		return ""
	}
	return contexts[0].PromptBlock()
}

func conflictingImageSuggestion(post, author string) LeadSuggestion {
	reply := leadSalutation(author) + ", nội dung chữ và ảnh chưa thống nhất về sản phẩm. Bạn xác nhận giúp mẫu cần tìm hoặc gửi mã/link sản phẩm nhé?"
	if supplierQueryLanguage(post) == "en" {
		reply = "The text and images appear to refer to different products. Which item should we use as the reference, or could you share its model or product link?"
	}
	return LeadSuggestion{Reply: reply, ContextNote: "Chữ và ảnh có điểm chưa khớp; cần xác nhận trước khi ghép sản phẩm hoặc báo giá."}
}
