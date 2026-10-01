package ai

import (
	"context"
	"fmt"
	"strings"
)

// Operator reply suggestions are a different product surface from the comment
// the runtime posts to Facebook: an operator reads this one in Telegram, judges
// it in a couple of seconds, and pastes it. So it gets its own prompt — short,
// concrete, and anchored to sourcing facts — instead of sharing the outbound
// comment prompt, whose length and CTA policy exist for a different job.

// LeadReplyRequest is the input to one operator reply suggestion. GroundedFacts
// is a pre-assembled block of REAL, already-retrieved product facts; the caller
// owns its assembly so this package never decides what counts as grounded.
type LeadReplyRequest struct {
	PostContent     string
	AuthorName      string
	BusinessContext string
	BrandName       string
	GroundedFacts   string
}

// GenerateLeadReplySuggestion writes the short reply an operator can send as-is.
//
// The prompt is deliberately strict about numbers: price, weight, MOQ and origin
// may only be repeated from GroundedFacts. A model that cannot find a number
// there must leave it out rather than estimate one — an invented shipping weight
// would become a quote the business cannot honour.
func (mg *MessageGenerator) GenerateLeadReplySuggestion(ctx context.Context, req LeadReplyRequest) (string, error) {
	langRule := "Viết bằng tiếng Việt, xưng hô tự nhiên."
	if detectLang(req.PostContent) == "en" {
		langRule = "MUST write in English."
	}
	brand := strings.TrimSpace(req.BrandName)
	brandRule := "Name your business exactly as it appears in the BUSINESS PROFILE."
	if brand != "" {
		brandRule = fmt.Sprintf("Refer to your business as %q — exactly this name.", brand)
	}
	facts := strings.TrimSpace(req.GroundedFacts)
	if facts == "" {
		facts = "(no product facts available — do not mention any product, price, weight or MOQ)"
	}

	prompt := fmt.Sprintf(`Write a short reply an experienced salesperson would send to the author of this post.

BUSINESS PROFILE:
%s

GROUNDED PRODUCT FACTS (the ONLY source for any product name, price, weight, MOQ or origin):
%s

POST AUTHOR: %s
POST CONTENT:
"""%s"""

SHAPE (follow it, do not label the parts):
1. Greet the author by their EXACT name.
2. One short clause showing you read what they actually need.
3. State the matched product, its supplier-source price labelled as a reference (if available), and the reference shipping per parcel (if available). Never call a supplier-source price THG's selling price.
4. Include the matched product link exactly as supplied and one soft closing question.

RULES:
- %s
- %s
- Maximum 2 short sentences. Keep the product link verbatim.
- Every number and every link must come from GROUNDED PRODUCT FACTS, copied exactly. Never estimate, convert, or round a price or weight.
- If a fact is missing from that block, simply omit it. Never write a placeholder.
- No emojis. No marketing superlatives. No bullet points.
- Transit, if present, is a reference for one parcel. Never imply that it covers a bulk order.
- Do not promise stock levels or discounts.

RETURN ONLY THE REPLY, NO EXPLANATION.`, req.BusinessContext, facts, req.AuthorName, req.PostContent, langRule, brandRule)

	return mg.callOpenAI(ctx, prompt)
}
