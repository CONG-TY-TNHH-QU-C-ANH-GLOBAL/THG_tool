// Package media validates references to original Facebook post photos.
// It never downloads media or supplies cookies to a model provider.
package media

import (
	"net/url"
	"strings"
)

// URLs admits up to three distinct HTTPS Facebook CDN references. Signed query
// parameters are retained for retrieval, never logged or used as source prices.
func URLs(raw []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if len(value) > 4096 || seen[value] || !facebookImageURL(value) {
			continue
		}
		out = append(out, value)
		seen[value] = true
		if len(out) == 3 {
			break
		}
	}
	return out
}

func facebookImageURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.Path == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "fbcdn.net" || strings.HasSuffix(host, ".fbcdn.net") || host == "fbsbx.com" || strings.HasSuffix(host, ".fbsbx.com")
}
