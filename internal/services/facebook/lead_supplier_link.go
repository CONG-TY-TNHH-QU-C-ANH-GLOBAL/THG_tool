package facebook

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/thg/scraper/internal/suppliersourcing"
)

var leadURLPattern = regexp.MustCompile(`https://[^\s<>"']+`)
var offerPathID = regexp.MustCompile(`(?i)/offer/(\d+)`)

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
	if validHTTPSURL(raw) == "" {
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
	if validHTTPSURL(raw) == "" {
		return false
	}
	u, _ := url.Parse(raw)
	host := strings.ToLower(u.Hostname())
	return host == "1688.com" || strings.HasSuffix(host, ".1688.com") ||
		host == "taobao.com" || strings.HasSuffix(host, ".taobao.com") ||
		host == "tmall.com" || strings.HasSuffix(host, ".tmall.com")
}

// A full pasted link is exact only when its marketplace item ID matches.
func sameMarketplaceListing(requested, returned string) bool {
	if !marketplaceURL(requested) || !marketplaceURL(returned) {
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

var numericListingID = regexp.MustCompile(`^\d+$`)

// Pricing Hub resolves a trusted marketplace short link to a canonical item.
// A short URL has no visible ID, so verify the destination marketplace and
// that the resolved link names a real item (not "offer/undefined.html").
func linkedMarketplaceListing(requested, returned string) bool {
	if sameMarketplaceListing(requested, returned) {
		return true
	}
	if !shortMarketplaceURL(requested) || !marketplaceURL(returned) {
		return false
	}
	a, _ := url.Parse(requested)
	b, _ := url.Parse(returned)
	return marketplacePlatform(a.Hostname()) == marketplacePlatform(b.Hostname()) &&
		numericListingID.MatchString(marketplaceListingID(b))
}
