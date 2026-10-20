package facebook

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/suppliersourcing"
)

type SupplierLookupFunc func(context.Context, string) (*ResolvedSupplier, error)

var errSupplierNoMatch = errors.New("supplier item did not match the requested listing")

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
		replyLang := supplierQueryLanguage(leadText)
		lang := replyLang
		if asciiQuery(query) {
			lang = "en"
		}
		if link, platform := leadMarketplaceURL(leadText); link != "" {
			return lookupLinkedSupplier(ctx, client, link, platform, lang, replyLang, leadQuantity(leadText))
		}
		if query == "" {
			return nil, nil
		}
		search := supplierSearch{client: client, query: query, lang: lang, replyLang: replyLang, quantity: leadQuantity(leadText)}
		return search.lookup(ctx, supplierLookupPlatforms(leadText))
	}
}

func lookupLinkedSupplier(ctx context.Context, client supplierReader, link, platform, lang, replyLang string, quantity int) (*ResolvedSupplier, error) {
	if err := supplierLookupBudget(ctx, 5*time.Second); err != nil {
		return nil, err
	}
	product, err := client.DetailLocalized(ctx, platform, "", link, lang)
	if err != nil {
		return nil, err
	}
	if product == nil || !linkedMarketplaceListing(link, product.Link) {
		return nil, errSupplierNoMatch
	}
	resolved, err := resolvedSupplier(product, platform, quantity, replyLang)
	if err != nil {
		return nil, errSupplierNoMatch
	}
	// The lead pasted this exact listing, so it is not a lookalike.
	resolved.Match.Similar = false
	return resolved, nil
}

func resolvedSupplier(product *suppliersourcing.Product, platform string, quantity int, replyLang string) (*ResolvedSupplier, error) {
	if product == nil || !marketplaceProductURL(product.Link) || product.Price == nil || *product.Price <= 0 {
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
		PriceText: supplierSourcePriceText(product, platform, quantity, replyLang), WeightKG: formatWeight(product.WeightKG),
		MOQText: formatMOQ(product.MOQ, product.Unit), ShipFrom: strings.TrimSpace(product.ShipFrom),
		ShopName: strings.TrimSpace(product.ShopName), CapturedAt: strings.TrimSpace(product.FetchedAt),
		Similar: true,
	}, WeightKG: product.WeightKG}, nil
}
