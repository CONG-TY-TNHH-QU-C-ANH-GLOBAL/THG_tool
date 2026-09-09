package facebook

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/ai"
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
	if builder == nil || msgGen == nil || !msgGen.Available() || profile == nil {
		return LeadSuggestion{}
	}
	candidates, _, err := builder.CandidatesForLead(ctx, orgID, leadText)
	if err != nil {
		return LeadSuggestion{}
	}
	product := PickSuggestedProductDetails(candidates)
	supplier := PickSuggestedSupplier(candidates)
	// Either grounding is enough to be worth notifying on: the workspace may have
	// only a catalog, only a sourced marketplace index, or both.
	if product.Name == "" && product.URL == "" && !supplier.HasOffer() {
		return LeadSuggestion{}
	}
	out := LeadSuggestion{
		ProductName: product.Name, ProductURL: product.URL, ProductImageURL: product.ImageURL,
		Supplier: supplier,
	}
	reply, err := msgGen.GenerateLeadReplySuggestion(ctx, ai.LeadReplyRequest{
		PostContent:     leadText,
		AuthorName:      author,
		BusinessContext: profile.ToPromptBlock(),
		BrandName:       profile.Name,
		GroundedFacts:   BuildGroundedFacts(product, supplier),
	})
	if err != nil {
		return out
	}
	out.Reply = strings.TrimSpace(reply)
	return out
}
