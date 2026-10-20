package facebook

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// LeadSuggestion is the operator-facing reply suggestion attached to a new-lead
// Telegram notice. Any field may be empty (suggestions are best-effort + opt-in).
type LeadSuggestion = models.LeadSuggestion

type ShippingQuoteFunc func(context.Context, models.ShippingRequest) (*models.ShippingReference, error)

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
		link := catalogProductURL(c.SourceURL)
		if link == "" || strings.TrimSpace(c.Title) == "" {
			continue
		}
		return SuggestedProduct{
			Name: strings.TrimSpace(c.Title), URL: link, ImageURL: validHTTPSURL(c.ImageURL),
		}
	}
	return SuggestedProduct{}
}

var catalogLocalePath = regexp.MustCompile(`(?i)^[a-z]{2}(?:-[a-z]{2})?$`)

// Catalog ownership comes from the approved tenant asset, not a global domain
// allowlist. Reject obvious index pages; this does not verify remote stock or SKU.
func catalogProductURL(raw string) string {
	link := validHTTPSURL(raw)
	if link == "" || marketplaceURL(link) || shortMarketplaceURL(link) {
		return ""
	}
	u, _ := url.Parse(link)
	path := strings.Trim(strings.ToLower(u.Path), "/")
	if path == "" || catalogLocalePath.MatchString(path) {
		return ""
	}
	parts := strings.Split(path, "/")
	switch parts[len(parts)-1] {
	case "catalog", "products", "collections":
		id := strings.TrimSpace(u.Query().Get("productId"))
		switch strings.ToLower(id) {
		case "", "undefined", "null":
			return ""
		}
	}
	return link
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
