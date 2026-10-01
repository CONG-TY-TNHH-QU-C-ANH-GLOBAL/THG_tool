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

// NewSupplierLookup makes at most two keyword searches and one detail request
// per marketplace. A failure on the first marketplace can fall back to the
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
		lang := "vi"
		if supplierEnglishQuery(leadText) != "" {
			lang = "en"
		}
		if link, platform := leadMarketplaceURL(leadText); link != "" {
			product, err := client.DetailLocalized(ctx, platform, "", link, lang)
			if err != nil {
				return nil, err
			}
			if query != "" && (product == nil || !matchesLeadProduct(query, product.Title)) {
				return nil, errors.New("linked supplier product did not match the lead")
			}
			return resolvedSupplier(product, platform)
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
		for _, platform := range platforms {
			items, err := client.SearchLocalized(ctx, query, platform, 10, lang)
			if err != nil {
				lookupErr = err
				continue
			}
			for _, item := range items {
				if !matchesLeadProduct(query, item.Title) || !marketplaceURL(item.Link) {
					continue
				}
				product, err := client.DetailLocalized(ctx, platform, item.ID, item.Link, lang)
				if err != nil {
					lookupErr = err
					break
				}
				if product == nil || !matchesLeadProduct(query, product.Title) {
					lookupErr = errors.New("supplier detail did not match the lead")
					break
				}
				resolved, err := resolvedSupplier(product, platform)
				if err == nil {
					return resolved, nil
				}
				lookupErr = err
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

// supplierQuery extracts the product phrase only when the post contains a
// purchase verb. This deliberately returns no query for vague logistics posts.
func supplierQuery(raw string) string {
	text := strings.ToLower(raw)
	if len([]rune(text)) > 1500 {
		text = string([]rune(text)[:1500])
	}
	for _, marker := range []string{"cần nhập ", "muốn nhập ", "cần tìm nguồn ", "cần nguồn ", "tìm nguồn ", "cần mua ", "muốn mua ", "nhập ", "mua "} {
		if at := strings.Index(text, marker); at >= 0 {
			text = text[at+len(marker):]
			goto cut
		}
	}
	return supplierEnglishQuery(raw)
cut:
	for _, marker := range []string{" từ 1688", " từ taobao", " về mỹ", " sang mỹ", " đi mỹ", " ship ", " khoảng ", " số lượng ", " mẫu này", " https://", " http://", "\n", ".", ",", ";"} {
		if at := strings.Index(text, marker); at >= 0 {
			text = text[:at]
		}
	}
	text = strings.TrimSpace(text)
	text = supplierLeadingQuantity.ReplaceAllString(text, "")
	for _, prefix := range []string{"một số lượng lớn ", "số lượng lớn ", "sản phẩm ", "hàng "} {
		text = strings.TrimPrefix(text, prefix)
	}
	words := strings.Fields(text)
	if len(words) < 2 || len(words) > 12 {
		return ""
	}
	return strings.Join(words, " ")
}

var supplierLeadingQuantity = regexp.MustCompile(`^\d{1,6}\s*(hộp|cái|chiếc|sản phẩm|pcs|units|đôi|boxes|pieces)\s+`)
