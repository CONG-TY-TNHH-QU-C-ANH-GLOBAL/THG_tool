package facebook

import (
	"context"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/platform/facebook/media"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// BuildLeadSuggestionWithContext is the opt-in need-analysis workflow. Numeric
// fields, links, pricing and company capability still come from their owners.
func BuildLeadSuggestionWithContext(ctx context.Context, builder *knowledgeRuntime.Builder, generator *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, post, author string, images []string, shipping ShippingQuoteFunc, sourcing SupplierLookupFunc) LeadSuggestion {
	post = leadPostBody(post)
	urls := media.URLs(images)
	need, err := generator.AnalyzeLeadContext(ctx, post, urls)
	if err != nil {
		out := BuildLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, shipping, sourcing)
		out.AnalysisStatus = "unavailable"
		out.ReplyNote = "Chưa phân tích được đầy đủ ngữ cảnh; gợi ý dự phòng dựa trên nội dung chữ."
		return out
	}
	if need.Visual.Conflict {
		out := conflictingImageSuggestion(post, author)
		out.AnalysisStatus = "conflict"
		return out
	}
	out := groundedForLeadContext(ctx, builder, orgID, post, author, need, shipping, sourcing)
	out.AnalysisStatus = need.Intent
	if len(urls) > 0 {
		out.ContextNote = "Đã phân tích ảnh tham chiếu; chưa xác minh đúng mã hàng hoặc mẫu sàn."
	} else if len(images) > 0 {
		out.ContextNote = "Chưa nhận được ảnh gốc hợp lệ; gợi ý dựa trên chữ, sale cần đối chiếu ảnh."
	}
	visual := ""
	if len(urls) > 0 {
		visual = need.Visual.PromptBlock()
	}
	return refineLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, out, need.PromptBlock(), visual)
}

func groundedForLeadContext(ctx context.Context, builder *knowledgeRuntime.Builder, orgID int64, post, author string, need leadcontext.Context, shipping ShippingQuoteFunc, sourcing SupplierLookupFunc) LeadSuggestion {
	switch need.Intent {
	case leadcontext.NonCommercial:
		return LeadSuggestion{ReplyNote: "Bài chưa thể hiện nhu cầu mua hàng hoặc sử dụng dịch vụ phù hợp; sale cần xem trước khi tiếp cận."}
	case leadcontext.Shipping:
		return serviceSuggestionForKind(post, author, serviceExpress, need.Product != "")
	case leadcontext.Warehouse:
		return serviceSuggestionForKind(post, author, serviceWarehousing, need.Product != "")
	case leadcontext.Unclear:
		out := buildGroundedLeadSuggestion(ctx, builder, orgID, post, author, shipping, sourcing)
		out.ReplyNote = "Nhu cầu trong bài chưa rõ; sale cần xác nhận trước khi tư vấn."
		return out
	}
	return buildGroundedLeadSuggestion(ctx, builder, orgID, leadContextSearchText(post, need), author, shipping, sourcing)
}

func leadContextSearchText(post string, need leadcontext.Context) string {
	link, _ := leadMarketplaceURL(post)
	// An explicit item/link outranks a generated category hint. Original text
	// is retained below the hint so volume, destination and POD markers survive.
	if link != "" || supplierQuery(post) != "" {
		return post
	}
	if need.Product == "" {
		if supplierQueryLanguage(post) == "en" {
			return "Looking for a supplier.\n" + post
		}
		return "Cần nhà cung cấp.\n" + post
	}
	if supplierQueryLanguage(post) == "en" {
		prefix := "Looking for a supplier for " + need.Product
		if need.Intent == leadcontext.POD {
			prefix += " with custom logo"
		}
		return safeLeadRetrievalHint(post, prefix+".\n"+post)
	}
	prefix := "Cần tìm nguồn " + need.Product
	if need.Intent == leadcontext.POD {
		prefix += " in logo"
	}
	return safeLeadRetrievalHint(post, prefix+".\n"+post)
}

// Even a letters-only category can contain "one box" or "to US". It must not
// inject a quantity or route into the downstream deterministic parsers.
func safeLeadRetrievalHint(post, hinted string) string {
	if leadQuantity(hinted) != leadQuantity(post) || leadDestinationCountry(hinted) != leadDestinationCountry(post) {
		return post
	}
	return hinted
}
