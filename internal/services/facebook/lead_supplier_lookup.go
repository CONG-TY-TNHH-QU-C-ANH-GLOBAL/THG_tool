package facebook

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

type SupplierLookupFunc func(context.Context, string) (*ResolvedSupplier, error)

// ResolvedSupplier keeps the raw weight separate from the display string so
// the rate calculator never parses text meant for Telegram.
type ResolvedSupplier struct {
	Match    *models.SupplierMatch
	WeightKG *float64
}

type supplierReader interface {
	Search(context.Context, string, string, int) ([]suppliersourcing.SearchItem, error)
	Detail(context.Context, string, string, string) (*suppliersourcing.Product, error)
	SearchLocalized(context.Context, string, string, int, string) ([]suppliersourcing.SearchItem, error)
	DetailLocalized(context.Context, string, string, string, string) (*suppliersourcing.Product, error)
}

// NewSupplierLookup makes at most two keyword searches and two detail requests
// in total. A failure on the first marketplace can fall back to the
// second when the post did not name a specific marketplace.
// A product URL already present in the post goes straight to detail, avoiding
// a search that could select a different seller's product.
// Pricing Hub owns caching and quota accounting. Results without a matching
// title and a canonical marketplace link are never suggested.
func NewSupplierLookup(client supplierReader) SupplierLookupFunc {
	if client == nil {
		return nil
	}
	return func(ctx context.Context, leadText string) (*ResolvedSupplier, error) {
		query := supplierQuery(leadText)
		lang := supplierQueryLanguage(leadText)
		if link, platform := leadMarketplaceURL(leadText); link != "" {
			product, err := client.DetailLocalized(ctx, platform, "", link, lang)
			if err != nil {
				return nil, err
			}
			if product == nil || !sameMarketplaceListing(link, product.Link) {
				return nil, errors.New("linked supplier product resolved to a different listing")
			}
			if query != "" && !matchesSupplierQuery(query, product.Title) {
				return nil, errors.New("linked supplier product did not match the lead")
			}
			resolved, err := resolvedSupplier(product, platform)
			if resolved != nil {
				// The lead pasted this exact listing, so it is not a lookalike.
				resolved.Match.Similar = false
			}
			return resolved, err
		}
		if query == "" {
			return nil, nil
		}
		platforms := []string{suppliersourcing.PlatformAlibaba, suppliersourcing.PlatformTaobao}
		lowerLead := strings.ToLower(leadText)
		if strings.Contains(lowerLead, "1688") && !strings.Contains(lowerLead, "taobao") {
			platforms = platforms[:1]
		} else if strings.Contains(lowerLead, "taobao") && !strings.Contains(lowerLead, "1688") {
			platforms = []string{suppliersourcing.PlatformTaobao}
		}
		var lookupErr error
		detailAttempts := 0
		for _, platform := range platforms {
			items, err := client.SearchLocalized(ctx, query, platform, 10, lang)
			if err != nil {
				lookupErr = err
				continue
			}
			for _, item := range items {
				if !matchesSupplierQuery(query, item.Title) || !marketplaceURL(item.Link) {
					continue
				}
				if detailAttempts >= 2 {
					break
				}
				detailAttempts++
				product, err := client.DetailLocalized(ctx, platform, item.ID, item.Link, lang)
				if err != nil {
					lookupErr = err
					continue
				}
				if product == nil || !matchesSupplierQuery(query, product.Title) {
					lookupErr = errors.New("supplier detail did not match the lead")
					continue
				}
				resolved, err := resolvedSupplier(product, platform)
				if err == nil {
					return resolved, nil
				}
				lookupErr = err
			}
			if detailAttempts >= 2 {
				break
			}
		}
		return nil, lookupErr
	}
}

func resolvedSupplier(product *suppliersourcing.Product, platform string) (*ResolvedSupplier, error) {
	if product == nil || !marketplaceURL(product.Link) || product.Price == nil || *product.Price <= 0 {
		return nil, errors.New("supplier detail has no usable link or price")
	}
	name := strings.TrimSpace(product.Title)
	if name == "" {
		return nil, errors.New("supplier detail has no product title")
	}
	label := "1688"
	if platform == suppliersourcing.PlatformTaobao {
		label = "Taobao"
	}
	image := ""
	if len(product.Images) > 0 {
		image = validHTTPSURL(product.Images[0])
	}
	return &ResolvedSupplier{Match: &models.SupplierMatch{
		Platform: label, Name: name, URL: product.Link, ImageURL: image,
		PriceText: formatYuan(product.Price), WeightKG: formatWeight(product.WeightKG),
		MOQText: formatMOQ(product.MOQ, product.Unit), ShipFrom: strings.TrimSpace(product.ShipFrom),
		ShopName: strings.TrimSpace(product.ShopName), CapturedAt: strings.TrimSpace(product.FetchedAt),
		Similar: true,
	}, WeightKG: product.WeightKG}, nil
}

var leadURLPattern = regexp.MustCompile(`https://[^\s<>"']+`)

func leadMarketplaceURL(text string) (string, string) {
	for _, raw := range leadURLPattern.FindAllString(text, 4) {
		link := strings.TrimRight(raw, ".,;!?)]}")
		if !marketplaceURL(link) {
			continue
		}
		u, _ := url.Parse(link)
		host := strings.ToLower(u.Hostname())
		if host == "1688.com" || strings.HasSuffix(host, ".1688.com") {
			return link, suppliersourcing.PlatformAlibaba
		}
		return link, suppliersourcing.PlatformTaobao
	}
	return "", ""
}

func marketplaceURL(raw string) bool {
	if validHTTPSURL(raw) == "" {
		return false
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	return host == "1688.com" || strings.HasSuffix(host, ".1688.com") ||
		host == "taobao.com" || strings.HasSuffix(host, ".taobao.com") ||
		host == "tmall.com" || strings.HasSuffix(host, ".tmall.com")
}

var offerPathID = regexp.MustCompile(`(?i)/offer/(\d+)`)

// A pasted link is an exact offer only when Pricing Hub returns the same item ID.
// Tracking parameters and mobile/desktop hosts may differ without changing it.
func sameMarketplaceListing(requested, returned string) bool {
	if !marketplaceURL(requested) || !marketplaceURL(returned) {
		return false
	}
	a, _ := url.Parse(requested)
	b, _ := url.Parse(returned)
	platform := func(host string) string {
		host = strings.ToLower(host)
		if host == "1688.com" || strings.HasSuffix(host, ".1688.com") {
			return "1688"
		}
		return "taobao"
	}
	if platform(a.Hostname()) != platform(b.Hostname()) {
		return false
	}
	id := func(u *url.URL) string {
		if found := offerPathID.FindStringSubmatch(u.Path); len(found) > 1 {
			return found[1]
		}
		if value := u.Query().Get("offerId"); value != "" {
			return value
		}
		return u.Query().Get("id")
	}
	return id(a) != "" && id(a) == id(b)
}
