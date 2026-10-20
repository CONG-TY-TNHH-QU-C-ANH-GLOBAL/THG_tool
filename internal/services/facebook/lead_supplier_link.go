package facebook

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/suppliersourcing"
)

var leadURLPattern = regexp.MustCompile(`https://[^\s<>"']+`)
var offerPathID = regexp.MustCompile(`(?i)^/offer/([1-9]\d*)(?:\.html)?/?$`)

func leadMarketplaceURL(text string) (string, string) {
	for _, raw := range leadURLPattern.FindAllString(text, 4) {
		link := strings.TrimRight(raw, ".,;!?)]}")
		if !marketplaceURL(link) && !shortMarketplaceURL(link) {
			continue
		}
		u, _ := url.Parse(link)
		if marketplacePlatform(u.Hostname()) == suppliersourcing.PlatformAlibaba {
			return link, suppliersourcing.PlatformAlibaba
		}
		return link, suppliersourcing.PlatformTaobao
	}
	return "", ""
}

func marketplacePlatform(host string) string {
	host = strings.ToLower(host)
	if host == "1688.com" || strings.HasSuffix(host, ".1688.com") {
		return suppliersourcing.PlatformAlibaba
	}
	return suppliersourcing.PlatformTaobao
}

func shortMarketplaceURL(raw string) bool {
	raw = validHTTPSURL(raw)
	if raw == "" {
		return false
	}
	u, _ := url.Parse(raw)
	switch strings.ToLower(u.Hostname()) {
	case "tb.cn", "e.tb.cn", "m.tb.cn", "qr.1688.com", "s.click.taobao.com":
		return true
	}
	return false
}

func marketplaceURL(raw string) bool {
	raw = validHTTPSURL(raw)
	if raw == "" {
		return false
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	return host == "1688.com" || strings.HasSuffix(host, ".1688.com") ||
		host == "taobao.com" || strings.HasSuffix(host, ".taobao.com") ||
		host == "tmall.com" || strings.HasSuffix(host, ".tmall.com")
}

// Only canonical item links can become offers. Search/shop pages and trusted
// short links may be lookup inputs, but must first resolve to an actual item.
func marketplaceProductURL(raw string) bool {
	if raw != validHTTPSURL(raw) || !marketplaceURL(raw) {
		return false
	}
	u, _ := url.Parse(raw)
	if marketplacePlatform(u.Hostname()) == suppliersourcing.PlatformAlibaba {
		return offerPathID.MatchString(u.Path)
	}
	switch strings.ToLower(u.Path) {
	case "/item.htm", "/item.html", "/awp/core/detail.htm", "/detail/detail.html":
		return numericListingID.MatchString(u.Query().Get("id"))
	}
	return false
}

// A full pasted link is exact only when its marketplace item ID matches.
func sameMarketplaceListing(requested, returned string) bool {
	if !marketplaceProductURL(requested) || !marketplaceProductURL(returned) {
		return false
	}
	a, _ := url.Parse(requested)
	b, _ := url.Parse(returned)
	if marketplacePlatform(a.Hostname()) != marketplacePlatform(b.Hostname()) {
		return false
	}
	return marketplaceListingID(a) != "" && marketplaceListingID(a) == marketplaceListingID(b)
}

func marketplaceListingID(u *url.URL) string {
	if found := offerPathID.FindStringSubmatch(u.Path); len(found) > 1 {
		return found[1]
	}
	if value := u.Query().Get("offerId"); value != "" {
		return value
	}
	return u.Query().Get("id")
}

var numericListingID = regexp.MustCompile(`^[1-9]\d*$`)

// Pricing Hub resolves a trusted marketplace short link to a canonical item.
// A short URL has no visible ID, so verify the destination marketplace and
// that the resolved link names a real item (not "offer/undefined.html").
func linkedMarketplaceListing(requested, returned string) bool {
	if sameMarketplaceListing(requested, returned) {
		return true
	}
	if !shortMarketplaceURL(requested) || !marketplaceProductURL(returned) {
		return false
	}
	a, _ := url.Parse(requested)
	b, _ := url.Parse(returned)
	return marketplacePlatform(a.Hostname()) == marketplacePlatform(b.Hostname()) &&
		numericListingID.MatchString(marketplaceListingID(b))
}
