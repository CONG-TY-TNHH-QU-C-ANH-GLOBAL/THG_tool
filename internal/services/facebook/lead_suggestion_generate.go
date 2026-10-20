package facebook

import (
	"context"
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/ai/visualcontext"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// BuildLeadSuggestion keeps fact selection independent from wording. A failed
// generator cannot remove the operator's grounded draft or change quote data.
func BuildLeadSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, msgGen *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, leadText, author string, shippingQuote ShippingQuoteFunc, supplierLookup SupplierLookupFunc, visual ...visualcontext.Context) LeadSuggestion {
	leadText = leadPostBody(leadText)
	out := buildGroundedLeadSuggestion(ctx, builder, orgID, imageAwareLeadText(leadText, visual), author, shippingQuote, supplierLookup)
	return refineLeadSuggestion(ctx, builder, msgGen, profile, orgID, leadText, author, out, "", visualContextBlock(visual))
}

func refineLeadSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, msgGen *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, post, author string, out LeadSuggestion, need, visual string) LeadSuggestion {
	out.ReplyStatus = "grounded_template"
	if out.Reply == "" {
		out.ReplyStatus = "not_applicable"
		return out
	}
	if msgGen == nil || !msgGen.Available() || profile == nil {
		out.ReplyStatus = "provider_or_profile_unavailable"
		return out
	}
	// Supplier rewriting is opt-in through the analyzed-context pipeline.
	if out.Supplier.HasOffer() && need == "" {
		return out
	}
	knowledge, _ := builder.ReplyKnowledgeForLead(ctx, orgID, post)
	if knowledge == "" && out.ProductURL == "" && !out.Supplier.HasOffer() {
		out.ReplyStatus = "no_public_knowledge"
		return out
	}
	facts := BuildGroundedFacts(SuggestedProduct{Name: out.ProductName, URL: out.ProductURL}, out.Supplier)
	reply, err := msgGen.GenerateLeadReplySuggestion(ctx, ai.LeadReplyRequest{
		PostContent: post, AuthorName: author, BusinessContext: profile.ToPromptBlock(),
		BrandName: profile.Name, GroundedFacts: facts, CompanyKnowledge: knowledge, GroundedDraft: out.Reply,
		VisualContext: visual, LeadContext: need,
	})
	reply = strings.TrimSpace(reply)
	if err != nil {
		out.ReplyStatus = "model_error"
		return out
	}
	selectedURL := out.ProductURL
	if out.Supplier.HasOffer() {
		selectedURL = out.Supplier.URL
	}
	reply = ensureSelectedReplyLink(reply, selectedURL)
	if !safeAnalyzedReply(reply, post, out) {
		out.ReplyStatus = "model_rejected"
		return out
	}
	if out.ProductURL != "" && len([]rune(reply)) > 320 {
		out.ReplyStatus = "model_rejected"
		return out
	}
	out.Reply = reply
	out.ReplyStatus = "model_generated"
	return out
}

func safeAnalyzedReply(reply, post string, out LeadSuggestion) bool {
	if out.Supplier.HasOffer() {
		custom := wantsPersonalizedPOD(post) || containsAnyLeadPhrase(out.Reply, []string{"confirm customization", "xác nhận khả năng in"})
		return safeSupplierReply(reply, post, out.Supplier, custom)
	}
	return safeKnowledgeReply(reply, post, out)
}

var replyNumber = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// This is a conservative syntax guard, not a semantic proof of correctness.
// The template remains the fallback; model quality also needs live review.
func safeKnowledgeReply(reply, post string, out LeadSuggestion) bool {
	if reply == "" || len([]rune(reply)) > 650 || !generatedPODReplyGrounded(reply, out.ProductURL) {
		return false
	}
	if unsupportedReplyClaim(reply) {
		return false
	}
	allowed := replyNumber.FindAllString(post+" "+out.ProductName, -1)
	for _, number := range replyNumber.FindAllString(podReplyURL.ReplaceAllString(reply, ""), -1) {
		if !containsReplyNumber(allowed, number) {
			return false
		}
	}
	return true
}

func containsReplyNumber(allowed []string, number string) bool {
	for _, value := range allowed {
		if value == number {
			return true
		}
	}
	return false
}
