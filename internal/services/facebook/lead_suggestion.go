package facebook

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/crmenrich"
	"github.com/thg/scraper/internal/models"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// Operator reply-suggestion helpers (Telegram lead notice). These are PURE:
// the composition root does the retrieval + generation IO and hands the results
// here; the sink only renders the assembled fields. No store, no network.

// LeadSuggestion is the operator-facing reply suggestion attached to a new-lead
// Telegram notice. Any field may be empty (suggestions are best-effort + opt-in).
type LeadSuggestion = models.LeadSuggestion

// OrgAllowlist is a fail-closed rollout policy. The wildcard is accepted only
// when it is the complete value; any malformed token invalidates the full list.
type OrgAllowlist struct {
	all bool
	ids map[int64]struct{}
}

func ParseOrgAllowlist(raw string) OrgAllowlist {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return OrgAllowlist{all: true}
	}
	if raw == "" {
		return OrgAllowlist{}
	}
	ids := make(map[int64]struct{})
	for _, token := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(token), 10, 64)
		if err != nil || id <= 0 {
			return OrgAllowlist{}
		}
		ids[id] = struct{}{}
	}
	return OrgAllowlist{ids: ids}
}

func (a OrgAllowlist) Allows(orgID int64) bool {
	if orgID <= 0 {
		return false
	}
	if a.all {
		return true
	}
	_, ok := a.ids[orgID]
	return ok
}

func (a OrgAllowlist) Configured() bool {
	return a.all || len(a.ids) > 0
}

type SuggestedProduct struct {
	Name, URL, ImageURL string
	// PriceText is the candidate's own formatted price ("4.5-9 USD"). Retrieval
	// has always computed it; carrying it here is what lets the reply name a
	// number instead of staying vague.
	PriceText string
}

func validHTTPSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.ParseRequestURI(raw)
	if err != nil || !u.IsAbs() || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return raw
}

// PickSuggestedProductDetails returns only persisted, safe-to-render catalog
// fields from the first ranked available product.
func PickSuggestedProductDetails(candidates []models.KnowledgeCandidate) SuggestedProduct {
	for _, c := range candidates {
		if c.Kind != "POD_product" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(c.Availability)) {
		case "out_of_stock", "discontinued":
			continue
		}
		link := validHTTPSURL(c.SourceURL)
		if link == "" {
			continue
		}
		return SuggestedProduct{
			Name: strings.TrimSpace(c.Title), URL: link, ImageURL: validHTTPSURL(c.ImageURL),
			PriceText: strings.TrimSpace(c.PriceText),
		}
	}
	return SuggestedProduct{}
}

// PickSuggestedProduct returns the first grounded POD-product candidate that
// carries a real catalog PDP link, as (name, url). Retrieval already ranked the
// candidates, so the first product with a link is the best-matched one. Returns
// ("","") when no product candidate has a link — the notice then omits the
// product block. Never fabricates: only a candidate's own Title/SourceURL is used.
func PickSuggestedProduct(candidates []models.KnowledgeCandidate) (name, url string) {
	p := PickSuggestedProductDetails(candidates)
	return p.Name, p.URL
}

// BuildLeadSuggestion performs the optional, operator-facing suggestion work.
// It is intentionally best-effort: retrieval or generation failure returns an
// empty suggestion and must never affect lead ingestion.
func BuildLeadSuggestion(ctx context.Context, builder *knowledgeRuntime.Builder, msgGen *ai.MessageGenerator, profile *ai.BusinessProfile, orgID int64, leadText, author string) LeadSuggestion {
	return BuildLeadSuggestionWithCRM(ctx, builder, msgGen, profile, nil, orgID, leadText, author, "", "")
}

// BuildLeadSuggestionWithCRM is BuildLeadSuggestion plus the numbers only the
// CRM knows: the price of a Chinese-marketplace product named in the post, and
// the shipping cost for it off a published rate card.
//
// `crm` may be nil and every call through it is best-effort. A lead notice must
// go out whether or not the CRM answers — without it the reply simply carries
// no figures, exactly as before.
func BuildLeadSuggestionWithCRM(
	ctx context.Context,
	builder *knowledgeRuntime.Builder,
	msgGen *ai.MessageGenerator,
	profile *ai.BusinessProfile,
	crm *crmenrich.Client,
	orgID int64,
	leadText, author, sourceURL, category string,
) LeadSuggestion {
	if builder == nil || msgGen == nil || !msgGen.Available() || profile == nil {
		return LeadSuggestion{}
	}
	candidates, _, err := builder.CandidatesForLead(ctx, orgID, leadText)
	if err != nil {
		return LeadSuggestion{}
	}
	product := PickSuggestedProductDetails(candidates)

	// The CRM lookup is keyed off the post text, not off the POD catalog match,
	// so it can fire even when nothing in our own catalog matched. That is the
	// case worth covering: a lead that pastes a 1688 link is asking about a
	// product we do not stock, and a real landed cost is the whole answer.
	var enriched crmenrich.Result
	if crm.Available() {
		enriched = crm.Enrich(ctx, leadText, sourceURL, category)
	}

	if product.Name == "" && product.URL == "" && !enriched.HasProduct() {
		return LeadSuggestion{}
	}
	// serviceMatch stays name+URL: it is keyword-matched to pick a THG service
	// link, not shown to the model. Price goes through ProductFacts instead —
	// mixing it in here would skew that keyword match and still never reach the
	// prompt, which is exactly why the number was being lost.
	serviceMatch := product.Name + " " + product.URL

	// The marketplace product the lead actually named wins over our catalog
	// match: it is what they asked about, and it is the one carrying a real
	// price and weight. Our catalog entry stays the fallback.
	facts := ai.ProductFacts{Name: product.Name, PriceText: product.PriceText, URL: product.URL}
	out := LeadSuggestion{ProductName: product.Name, ProductURL: product.URL, ProductImageURL: product.ImageURL}
	if enriched.HasProduct() {
		facts.Name = enriched.Product.Title
		facts.URL = enriched.Product.URL
		facts.PriceText = enriched.PriceText()
		out.ProductName = enriched.Product.Title
		out.ProductURL = enriched.Product.URL
		if img := strings.TrimSpace(enriched.Product.ImageURL); img != "" {
			out.ProductImageURL = validHTTPSURL(img)
		}
	}
	// Prompt gets the one-string form; the notice gets cost and basis separately
	// because it puts the basis on its own indented line under the number.
	facts.ShippingText = enriched.ShippingText()
	out.ShippingLine = enriched.ShippingCost()
	out.ShippingBasis = enriched.ShippingBasis()
	out.ProductLine = enriched.ProductLine()

	reply, err := msgGen.GenerateCommentWithProduct(ctx, leadText, author, profile.ToPromptBlock(), serviceMatch, facts, models.CompanyIdentity{}, models.ActorPersona{})
	if err != nil {
		return out
	}
	out.Reply = strings.TrimSpace(reply)
	return out
}
