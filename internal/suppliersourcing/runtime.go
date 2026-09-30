package suppliersourcing

import (
	"net/url"
	"os"
	"strings"
	"time"
)

const DefaultPricingHubURL = "https://pricingtool.thgfulfill.com"

// RuntimeClient uses the same Pricing Hub integration secret as supplier_catalog.
// An absent secret disables live lookup without affecting lead delivery.
func RuntimeClient() *Client {
	key := strings.TrimSpace(os.Getenv("PRICING_HUB_INTEGRATION_KEY"))
	if key == "" {
		data, err := os.ReadFile("/etc/thg-scraper/pricing_hub_key")
		if err == nil {
			key = strings.TrimSpace(string(data))
		}
	}
	base := strings.TrimSpace(os.Getenv("PRICING_HUB_BASE_URL"))
	if base == "" {
		base = DefaultPricingHubURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return nil
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "pricingtool.thgfulfill.com" && !strings.HasSuffix(host, ".thgfulfill.com") {
		return nil
	}
	return NewClient(base, key, 4*time.Second)
}
