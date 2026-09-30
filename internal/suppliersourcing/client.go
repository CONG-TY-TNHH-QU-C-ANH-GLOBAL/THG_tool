// Package suppliersourcing reads Taobao/1688 product facts from the THG
// Pricing Hub worker, which owns the upstream Elim API key, the shared D1
// response cache, and the shared request budget. This package never talks to
// the Chinese marketplaces or to Elim directly: keeping one caller means one
// cache and one place where the quota is observable.
package suppliersourcing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PlatformTaobao and PlatformAlibaba are the only two values the upstream API
// accepts; 1688 links are addressed as "alibaba".
const (
	PlatformTaobao  = "taobao"
	PlatformAlibaba = "alibaba"
)

const maxResponseBytes = 1 << 20

// Client is a thin, read-only reader over the pricing hub's scrape endpoints.
type Client struct {
	baseURL string
	key     string
	http    *http.Client
}

// NewClient returns nil when the integration is not configured, so every
// caller degrades to "no supplier facts" rather than to a runtime error.
func NewClient(endpoint, key string, timeout time.Duration) *Client {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	key = strings.TrimSpace(key)
	if endpoint == "" || key == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{baseURL: endpoint, key: key, http: &http.Client{Timeout: timeout}}
}

// NormalizePlatform maps the marketplace names operators use onto the two the
// upstream accepts. An unrecognized value returns "" so callers can reject it
// instead of silently querying the wrong marketplace.
func NormalizePlatform(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1688", "alibaba":
		return PlatformAlibaba
	case "taobao", "tmall":
		return PlatformTaobao
	default:
		return ""
	}
}

// Detail resolves one product. Pass productURL for Taobao whenever it is known:
// since 2026-08 Taobao rejects bare numeric item ids and the upstream needs the
// link (or an mi_id) to resolve them.
func (c *Client) Detail(ctx context.Context, platform, productID, productURL string) (*Product, error) {
	if c == nil {
		return nil, errors.New("suppliersourcing: client not configured")
	}
	pf := NormalizePlatform(platform)
	if pf == "" {
		return nil, fmt.Errorf("suppliersourcing: unsupported platform %q", platform)
	}
	body := map[string]any{"platform": pf, "lang": "vi"}
	if id := strings.TrimSpace(productID); id != "" {
		body["id"] = id
	}
	if link := strings.TrimSpace(productURL); link != "" {
		body["url"] = link
	}
	if body["id"] == nil && body["url"] == nil {
		return nil, errors.New("suppliersourcing: detail needs a product id or url")
	}
	var envelope struct {
		OK      bool     `json:"ok"`
		Error   string   `json:"error"`
		Product *Product `json:"product"`
	}
	if err := c.post(ctx, "/api/scrape/detail", body, &envelope); err != nil {
		return nil, err
	}
	if !envelope.OK || envelope.Product == nil {
		return nil, fmt.Errorf("suppliersourcing: detail rejected: %s", fallbackError(envelope.Error))
	}
	return envelope.Product, nil
}

// Search runs a keyword lookup. size is clamped upstream to 1..40.
func (c *Client) Search(ctx context.Context, query, platform string, size int) ([]SearchItem, error) {
	if c == nil {
		return nil, errors.New("suppliersourcing: client not configured")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("suppliersourcing: empty search query")
	}
	pf := NormalizePlatform(platform)
	if pf == "" {
		return nil, fmt.Errorf("suppliersourcing: unsupported platform %q", platform)
	}
	if size <= 0 {
		size = 20
	}
	var envelope struct {
		OK    bool         `json:"ok"`
		Error string       `json:"error"`
		Items []SearchItem `json:"items"`
	}
	body := map[string]any{"q": query, "platform": pf, "size": size, "lang": "vi"}
	if err := c.post(ctx, "/api/scrape/search", body, &envelope); err != nil {
		return nil, err
	}
	if !envelope.OK {
		return nil, fmt.Errorf("suppliersourcing: search rejected: %s", fallbackError(envelope.Error))
	}
	return envelope.Items, nil
}

// Quota reads the remaining upstream budget. Callers use it to refuse a bulk
// index run that would exhaust the plan.
func (c *Client) Quota(ctx context.Context) (Quota, error) {
	var out Quota
	if c == nil {
		return out, errors.New("suppliersourcing: client not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/scrape/quota", nil)
	if err != nil {
		return out, err
	}
	return out, c.do(req, &out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("suppliersourcing: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	req.Header.Set("accept", "application/json")
	req.Header.Set("x-thg-integration-key", c.key)
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("suppliersourcing: %s: %w", req.URL.Path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("suppliersourcing: read %s: %w", req.URL.Path, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("suppliersourcing: %s responded %d", req.URL.Path, response.StatusCode)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("suppliersourcing: decode %s: %w", req.URL.Path, err)
	}
	return nil
}

func fallbackError(message string) string {
	if strings.TrimSpace(message) == "" {
		return "unknown upstream error"
	}
	return message
}
