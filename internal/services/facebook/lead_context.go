package facebook

import (
	"context"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/leadcontext"
	"github.com/thg/scraper/internal/platform/facebook/media"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// BuildLeadSuggestionWithContext is the opt-in need-analysis workflow. Numeric
// fields, links, pricing and company capability still come from their owners.
func BuildLeadSuggestionWithContext(ctx context.Context, builder *knowledgeRuntime.Builder, generator *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, post, author string, images []string, shipping ShippingQuoteFunc, sourcing SupplierLookupFunc) (out LeadSuggestion) {
	post = leadPostBody(post)
	// A business description can coexist with a buyer need. Let the analyzer
	// interpret the whole post instead of rejecting it for an offer phrase.
	urls := media.URLs(images)
	imageStatus := pendingImageStatus(len(images), len(urls))
	defer func() {
		out.ImageInputs, out.ImageAnalysis = len(urls), imageStatus
	}()
	need, err := generator.AnalyzeLeadContext(ctx, post, urls)
	if err != nil {
		out := BuildLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, shipping, sourcing)
		out.AnalysisStatus = "unavailable"
		out.ReplyNote = "Chưa phân tích được đầy đủ ngữ cảnh; gợi ý dự phòng dựa trên nội dung chữ."
		return out
	}
	imageStatus = completedImageStatus(imageStatus, len(urls), len(need.Visual.Observations) > 0, need.Visual.Conflict)
	unconfirmedProduct := !supportedLeadProductHint(post, need)
	if unconfirmedProduct {
		need.Product = ""
	}
	if need.Visual.Conflict {
		out := conflictingImageSuggestion(post, author)
		out.AnalysisStatus = "conflict"
		return out
	}
	if need.Scope != nil {
		return scopeSourcingSuggestion(ctx, builder, orgID, post, author, need, sourcing)
	}
	if directProductRequest(post) && contradictsProductIntent(need.Intent) {
		out := buildGroundedLeadSuggestion(ctx, builder, orgID, post, author, shipping, sourcing)
		out.AnalysisStatus, out.ReplyStatus = "intent_conflict", "grounded_template"
		out.ReplyNote = "Phân tích nhu cầu chưa khớp yêu cầu sản phẩm rõ trong bài; gợi ý dựa trên nội dung chữ, sale cần đối chiếu."
		return out
	}
	out = groundedForLeadContext(ctx, builder, orgID, post, author, need, shipping, sourcing)
	out.AnalysisStatus = need.Intent
	if unconfirmedProduct {
		out.ReplyNote = "Chưa có căn cứ xác nhận loại sản phẩm model đề xuất; cần làm rõ sản phẩm, không tự chọn nguồn thay khách."
	}
	if observedProductWithoutOffer(need, len(urls), out) {
		out.Reply = proactiveSourcingReply(post)
	}
	if len(urls) > 0 {
		out.ContextNote = "Đã phân tích ảnh tham chiếu; chưa xác minh đúng mã hàng hoặc mẫu sàn."
		if len(need.Visual.Observations) == 0 {
			out.ContextNote = "Chưa xác nhận được nội dung ảnh; gợi ý dựa trên chữ, sale cần đối chiếu ảnh."
		}
	} else if len(images) > 0 {
		out.ContextNote = "Chưa nhận được ảnh gốc hợp lệ; gợi ý dựa trên chữ, sale cần đối chiếu ảnh."
	}
	visual := ""
	if len(urls) > 0 {
		visual = need.Visual.PromptBlock()
	}
	return refineLeadSuggestion(ctx, builder, generator, profile, orgID, post, author, out, need.PromptBlock(), visual)
}

func supportedLeadProductHint(post string, need leadcontext.Context) bool {
	if need.Product == "" || containsLeadPhrase(post, need.Product) || matchesSupplierQuery(need.Product, post) {
		return true
	}
	if supplierQuery(post) != "" {
		return false // Explicit text identity outranks a different photo category.
	}
	return need.Visual.Product == need.Product && len(need.Visual.Observations) > 0
}

// Only a direct buyer request can override a contradictory model verdict.
// Educational/conditional mentions and explicit logistics requests stay separate.
func directProductRequest(post string) bool {
	if serviceInquiryKind(post) != "" || containsAnyLeadPhrase(post, serviceOfferTerms) {
		return false
	}
	if supplierQuery(post) == "" && !wantsPersonalizedPOD(post) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(post))
	for _, prefix := range []string{"looking for ", "need ", "i need ", "we need ", "seeking ", "sourcing ", "cần ", "mình cần ", "shop mình cần ", "tôi cần ", "muốn mua ", "muốn nhập "} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func contradictsProductIntent(intent string) bool {
	switch intent {
	case leadcontext.NonCommercial, leadcontext.Shipping, leadcontext.Warehouse:
		return true
	}
	return false
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
