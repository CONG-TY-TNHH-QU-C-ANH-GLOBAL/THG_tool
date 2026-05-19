package rest_json

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thg/scraper/internal/workspace_knowledge/products"
)

// Config is the connection_config JSON shape for a SourceRESTJSON
// source. Unknown JSON keys are tolerated so a future field addition
// does not force every existing source row to migrate.
//
// All paths in FieldMap and Pagination are dot-paths into the JSON
// response: "data" descends one level; "pagination.pages" descends
// two; "" means "the value is not present in this upstream". Wildcards
// are NOT supported — see package doc.
type Config struct {
	BaseURL          string             `json:"base_url"`
	ExtractorVersion string             `json:"extractor_version"`
	Request          RequestConfig      `json:"request"`
	Auth             AuthConfig         `json:"auth"`
	Pagination       PaginationConfig   `json:"pagination"`
	DataPath         string             `json:"data_path"`
	FieldMap         FieldMap           `json:"field_map"`
	Availability     AvailabilityConfig `json:"availability"`
}

// RequestConfig carries HTTP knobs that are not part of the wire
// protocol per se — timeout, User-Agent. Defaults applied by Validate.
type RequestConfig struct {
	TimeoutSeconds int    `json:"timeout_seconds"`
	UserAgent      string `json:"user_agent"`
}

// AuthConfig discriminates on Type. Token / header values are pulled
// from environment variables, not stored in the config blob — secrets
// never touch the knowledge_sources table.
type AuthConfig struct {
	// Type is one of: "none" | "bearer" | "header".
	Type string `json:"type"`

	// TokenEnv: for "bearer" — env var name holding the token.
	TokenEnv string `json:"token_env,omitempty"`

	// HeaderName + ValueEnv: for "header" — sends ValueEnv's value
	// under HeaderName.
	HeaderName string `json:"header_name,omitempty"`
	ValueEnv   string `json:"value_env,omitempty"`
}

// PaginationConfig describes how to walk the endpoint.
type PaginationConfig struct {
	// Scheme: "none" (single fetch) | "page" (page+limit query params).
	Scheme string `json:"scheme"`

	// For "page": query param names and starting page.
	PageParam  string `json:"page_param,omitempty"`
	LimitParam string `json:"limit_param,omitempty"`
	LimitValue int    `json:"limit_value,omitempty"`
	StartPage  int    `json:"start_page,omitempty"`

	// TotalPagesPath: dot-path into the response where the total page
	// count lives. Optional — if empty, the adapter walks until it
	// receives an empty data array.
	TotalPagesPath string `json:"total_pages_path,omitempty"`

	// MaxPages is a safety ceiling that prevents an unbounded walk if
	// the upstream lies about TotalPagesPath or returns non-empty
	// data forever. Default 100.
	MaxPages int `json:"max_pages,omitempty"`
}

// FieldMap is the per-canonical-field source path. Each value is a
// dot-path into the item JSON object; "" means "this field is not
// available on this upstream" and the canonical field stays at its
// zero value.
//
// Required keys: SourceID, Name, SourceUpdatedAt. Adapters MUST emit
// CanonicalProducts that pass [products.CanonicalProduct.Validate],
// which requires these three; an empty mapping for them is a config
// error (caught at Validate).
type FieldMap struct {
	SourceID          string `json:"source_id"`
	DisplaySKU        string `json:"display_sku,omitempty"`
	VendorSKU         string `json:"vendor_sku,omitempty"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Category          string `json:"category,omitempty"`
	Origin            string `json:"origin,omitempty"`
	Sizes             string `json:"sizes,omitempty"`
	Colors            string `json:"colors,omitempty"`
	Tags              string `json:"tags,omitempty"`
	PriceMin          string `json:"price_min,omitempty"`
	PriceMax          string `json:"price_max,omitempty"`
	Currency          string `json:"currency,omitempty"`
	Images            string `json:"images,omitempty"`
	SourceURLTemplate string `json:"source_url_template,omitempty"` // e.g. "https://hub.example.com/p/{id}" — {id} substituted with SourceID
	SourceUpdatedAt   string `json:"source_updated_at"`
}

// AvailabilityConfig maps an upstream string field into the canonical
// Availability enum. If FromField is empty, every product gets
// Default (or AvailUnknown when Default is also empty).
type AvailabilityConfig struct {
	FromField string                          `json:"from_field,omitempty"`
	Map       map[string]products.Availability `json:"map,omitempty"`
	Default   products.Availability           `json:"default,omitempty"`
}

// ── Validation ────────────────────────────────────────────────────

// DefaultExtractorVersion is the version stamp the adapter applies
// when the config did not specify one. Bumped when the extraction
// logic in this package changes in a way that produces different
// canonical products from the same upstream bytes.
const DefaultExtractorVersion = "rest_json/v1"

// DefaultMaxPages caps an unbounded walk when neither
// TotalPagesPath nor an empty page tells us to stop. 100 is high
// enough for any realistic catalog (10k items at limit 100) but
// low enough to prevent a runaway loop.
const DefaultMaxPages = 100

// DefaultTimeoutSeconds is applied when RequestConfig.TimeoutSeconds
// is unset. Generous enough for slow upstreams without letting a
// hang stall the whole sync scheduler.
const DefaultTimeoutSeconds = 30

// Validate enforces the boundary invariants and fills in defaults.
// Called once when the adapter starts a sync; a config that fails
// here yields a permanent ingestion error so the operator sees the
// problem on the source row's health status.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("rest_json: nil config")
	}
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	if !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		return errors.New("rest_json: base_url must be an http(s) URL")
	}
	if strings.TrimSpace(c.ExtractorVersion) == "" {
		c.ExtractorVersion = DefaultExtractorVersion
	}
	if c.Request.TimeoutSeconds <= 0 {
		c.Request.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if strings.TrimSpace(c.Request.UserAgent) == "" {
		c.Request.UserAgent = "THGKnowledgeIngestor/1.0"
	}

	// Auth
	switch strings.ToLower(strings.TrimSpace(c.Auth.Type)) {
	case "", "none":
		c.Auth.Type = "none"
	case "bearer":
		c.Auth.Type = "bearer"
		if strings.TrimSpace(c.Auth.TokenEnv) == "" {
			return errors.New("rest_json: auth=bearer requires token_env")
		}
	case "header":
		c.Auth.Type = "header"
		if strings.TrimSpace(c.Auth.HeaderName) == "" || strings.TrimSpace(c.Auth.ValueEnv) == "" {
			return errors.New("rest_json: auth=header requires header_name and value_env")
		}
	default:
		return fmt.Errorf("rest_json: unknown auth type %q", c.Auth.Type)
	}

	// Pagination
	switch strings.ToLower(strings.TrimSpace(c.Pagination.Scheme)) {
	case "", "none":
		c.Pagination.Scheme = "none"
	case "page":
		c.Pagination.Scheme = "page"
		if strings.TrimSpace(c.Pagination.PageParam) == "" {
			c.Pagination.PageParam = "page"
		}
		if strings.TrimSpace(c.Pagination.LimitParam) == "" {
			c.Pagination.LimitParam = "limit"
		}
		if c.Pagination.LimitValue <= 0 {
			c.Pagination.LimitValue = 100
		}
		if c.Pagination.StartPage <= 0 {
			c.Pagination.StartPage = 1
		}
	default:
		return fmt.Errorf("rest_json: unknown pagination scheme %q", c.Pagination.Scheme)
	}
	if c.Pagination.MaxPages <= 0 {
		c.Pagination.MaxPages = DefaultMaxPages
	}

	// Field map: SourceID, Name, SourceUpdatedAt mandatory.
	if strings.TrimSpace(c.FieldMap.SourceID) == "" {
		return errors.New("rest_json: field_map.source_id is required")
	}
	if strings.TrimSpace(c.FieldMap.Name) == "" {
		return errors.New("rest_json: field_map.name is required")
	}
	if strings.TrimSpace(c.FieldMap.SourceUpdatedAt) == "" {
		return errors.New("rest_json: field_map.source_updated_at is required")
	}

	// Availability: default fallback if from_field unset.
	if c.Availability.Default == "" {
		c.Availability.Default = products.AvailUnknown
	}
	if !c.Availability.Default.IsKnown() {
		return fmt.Errorf("rest_json: availability.default %q is not a known canonical value", c.Availability.Default)
	}
	for k, v := range c.Availability.Map {
		if !v.IsKnown() {
			return fmt.Errorf("rest_json: availability.map[%q] = %q is not a known canonical value", k, v)
		}
	}

	return nil
}

// ParseConfig decodes the connection_config blob into a Config and
// validates it. Returns a permanent ingestion error on malformed
// JSON or invalid config so the dispatcher surfaces it as an error
// status, not a stale one (re-trying invalid config never helps).
func ParseConfig(raw json.RawMessage) (*Config, error) {
	c := &Config{}
	if len(raw) == 0 || string(raw) == "null" {
		return c, errors.New("rest_json: empty connection_config")
	}
	if err := json.Unmarshal(raw, c); err != nil {
		return c, fmt.Errorf("rest_json: parse connection_config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

// TimeoutDuration is a convenience accessor that returns the
// configured timeout as a time.Duration. Always non-zero after
// Validate.
func (c *Config) TimeoutDuration() time.Duration {
	if c.Request.TimeoutSeconds <= 0 {
		return DefaultTimeoutSeconds * time.Second
	}
	return time.Duration(c.Request.TimeoutSeconds) * time.Second
}
