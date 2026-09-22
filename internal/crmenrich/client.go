// Package crmenrich asks the CRM for the numbers this repo cannot compute:
// the price of a Chinese-marketplace product named in a lead, and the shipping
// cost for it.
//
// Why the CRM and not here: the published rate cards (USPS zones, epacket
// brackets, sea LCL by W/M) and the Elim product reader both already live over
// there, kept current by a cron and a shared cache. Re-implementing either in
// this repo would mean two copies of a price list, and two copies of a price
// list is one wrong quote waiting to be sent to a customer.
//
// Every call is best-effort and short-fused. A lead notification must go out
// whether or not this succeeds — an empty Result simply renders a notice with
// no numbers, exactly as before this package existed.
package crmenrich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Product mirrors the CRM's summarised catalog record. Zero values mean the
// marketplace did not publish that field; they are never guesses.
type Product struct {
	Title     string  `json:"title"`
	ShopName  string  `json:"shopName"`
	PriceCNY  float64 `json:"priceCny"`
	PriceNote string  `json:"priceNote"`
	MOQ       float64 `json:"moq"`
	Unit      string  `json:"unit"`
	WeightKg  float64 `json:"weightKg"`
	URL       string  `json:"url"`
	ImageURL  string  `json:"imageUrl"`
}

// Quote is a shipping cost the CRM computed from a published rate card.
type Quote struct {
	OK       bool    `json:"ok"`
	TotalUSD float64 `json:"totalUsd"`
	Currency string  `json:"currency"`
	Transit  string  `json:"transit"`
	Basis    string  `json:"basis"`
}

type Result struct {
	Excerpt string   `json:"excerpt"`
	Product *Product `json:"product"`
	Quote   *Quote   `json:"quote"`
	Notes   []string `json:"notes"`
}

// HasProduct reports whether a grounded product came back.
func (r Result) HasProduct() bool { return r.Product != nil && strings.TrimSpace(r.Product.Title) != "" }

// HasQuote reports whether a usable shipping cost came back.
func (r Result) HasQuote() bool { return r.Quote != nil && r.Quote.OK && r.Quote.TotalUSD > 0 }

// PriceText renders the product price the way the comment prompt wants it, or
// "" when the marketplace published no price.
func (r Result) PriceText() string {
	if !r.HasProduct() || r.Product.PriceCNY <= 0 {
		return ""
	}
	out := fmt.Sprintf("%g CNY", r.Product.PriceCNY)
	if note := strings.TrimSpace(r.Product.PriceNote); note != "" {
		out += " (" + note + ")"
	}
	return out
}

// ShippingText renders the shipping cost as one line, or "" when there is none.
func (r Result) ShippingText() string {
	if !r.HasQuote() {
		return ""
	}
	out := fmt.Sprintf("$%.2f", r.Quote.TotalUSD)
	if t := strings.TrimSpace(r.Quote.Transit); t != "" {
		out += " · " + t
	}
	if b := strings.TrimSpace(r.Quote.Basis); b != "" {
		out += " (" + b + ")"
	}
	return out
}

type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// New returns nil when the integration is not configured, so callers can keep
// a nil client and skip enrichment without branching on config everywhere.
func New(baseURL, key string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key = strings.TrimSpace(key)
	if baseURL == "" || key == "" {
		return nil
	}
	// 6s: this sits on the path to a lead notification. A slow CRM must cost
	// the notice its numbers, never its delivery.
	return &Client{baseURL: baseURL, key: key, http: &http.Client{Timeout: 6 * time.Second}}
}

func (c *Client) Available() bool { return c != nil }

// Enrich asks the CRM about one lead. It never returns an error the caller has
// to handle: a failure is an empty Result, which renders as "no numbers".
func (c *Client) Enrich(ctx context.Context, text, sourceURL, category string) Result {
	if c == nil {
		return Result{}
	}
	payload, err := json.Marshal(map[string]string{"text": text, "sourceUrl": sourceURL, "category": category})
	if err != nil {
		return Result{}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/integrations/enrich", bytes.NewReader(payload))
	if err != nil {
		return Result{}
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-thg-integration-key", c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}
	}
	var out Result
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}
	}
	return out
}
