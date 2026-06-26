package rest_json

import (
	"strings"
	"testing"

	"github.com/thg/scraper/internal/workspace_knowledge/products"
)

// validConfig returns the minimal Config that passes Validate, so each
// table case can mutate one field and assert that section in isolation.
func validConfig() *Config {
	return &Config{
		BaseURL: "https://hub.example.com",
		FieldMap: FieldMap{
			SourceID:        "id",
			Name:            "name",
			SourceUpdatedAt: "updated_at",
		},
	}
}

func TestValidate_DefaultsApplied(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.ExtractorVersion != DefaultExtractorVersion {
		t.Errorf("ExtractorVersion = %q, want %q", c.ExtractorVersion, DefaultExtractorVersion)
	}
	if c.Request.TimeoutSeconds != DefaultTimeoutSeconds {
		t.Errorf("TimeoutSeconds = %d, want %d", c.Request.TimeoutSeconds, DefaultTimeoutSeconds)
	}
	if c.Request.UserAgent == "" {
		t.Error("UserAgent default not applied")
	}
	if c.Auth.Type != "none" {
		t.Errorf("Auth.Type = %q, want none", c.Auth.Type)
	}
	if c.Pagination.Scheme != "none" {
		t.Errorf("Pagination.Scheme = %q, want none", c.Pagination.Scheme)
	}
	if c.Pagination.MaxPages != DefaultMaxPages {
		t.Errorf("MaxPages = %d, want %d", c.Pagination.MaxPages, DefaultMaxPages)
	}
	if c.Availability.Default != products.AvailUnknown {
		t.Errorf("Availability.Default = %q, want %q", c.Availability.Default, products.AvailUnknown)
	}
}

func TestValidate_BaseURL(t *testing.T) {
	for _, bad := range []string{"", "ftp://x", "example.com", "  "} {
		c := validConfig()
		c.BaseURL = bad
		if err := c.Validate(); err == nil {
			t.Errorf("base_url=%q: want error, got nil", bad)
		}
	}
	c := validConfig()
	c.BaseURL = "  https://x.test  " // trimmed then accepted
	if err := c.Validate(); err != nil {
		t.Errorf("trimmed base_url: unexpected error %v", err)
	}
	if c.BaseURL != "https://x.test" {
		t.Errorf("base_url not trimmed: %q", c.BaseURL)
	}
}

func TestValidateAuth(t *testing.T) {
	cases := []struct {
		name    string
		mut     func(*Config)
		wantErr bool
		wantTyp string
	}{
		{"empty->none", func(c *Config) { c.Auth.Type = "" }, false, "none"},
		{"BEARER normalized", func(c *Config) { c.Auth.Type = "BEARER"; c.Auth.TokenEnv = "T" }, false, "bearer"},
		{"bearer missing token", func(c *Config) { c.Auth.Type = "bearer" }, true, ""},
		{"header ok", func(c *Config) { c.Auth.Type = "header"; c.Auth.HeaderName = "X"; c.Auth.ValueEnv = "V" }, false, "header"},
		{"header missing parts", func(c *Config) { c.Auth.Type = "header"; c.Auth.HeaderName = "X" }, true, ""},
		{"unknown", func(c *Config) { c.Auth.Type = "oauth" }, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.mut(c)
			err := c.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && c.Auth.Type != tc.wantTyp {
				t.Errorf("Auth.Type = %q, want %q", c.Auth.Type, tc.wantTyp)
			}
		})
	}
}

func TestValidatePagination(t *testing.T) {
	c := validConfig()
	c.Pagination.Scheme = "PAGE"
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.Pagination.Scheme != "page" {
		t.Errorf("Scheme = %q, want page", c.Pagination.Scheme)
	}
	if c.Pagination.PageParam != "page" || c.Pagination.LimitParam != "limit" {
		t.Errorf("param defaults not applied: %+v", c.Pagination)
	}
	if c.Pagination.LimitValue != 100 || c.Pagination.StartPage != 1 {
		t.Errorf("page defaults not applied: %+v", c.Pagination)
	}

	bad := validConfig()
	bad.Pagination.Scheme = "cursor"
	if err := bad.Validate(); err == nil {
		t.Error("unknown scheme: want error, got nil")
	}
}

func TestValidateFieldMap(t *testing.T) {
	for _, field := range []string{"source_id", "name", "source_updated_at"} {
		c := validConfig()
		switch field {
		case "source_id":
			c.FieldMap.SourceID = " "
		case "name":
			c.FieldMap.Name = ""
		case "source_updated_at":
			c.FieldMap.SourceUpdatedAt = ""
		}
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("missing %s: want error mentioning it, got %v", field, err)
		}
	}
}

func TestValidateAvailability(t *testing.T) {
	c := validConfig()
	c.Availability.Default = products.AvailInStock
	c.Availability.Map = map[string]products.Availability{"y": products.AvailLowStock}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid availability rejected: %v", err)
	}

	badDefault := validConfig()
	badDefault.Availability.Default = "sold"
	if err := badDefault.Validate(); err == nil {
		t.Error("bad default: want error, got nil")
	}

	badMap := validConfig()
	badMap.Availability.Map = map[string]products.Availability{"y": "maybe"}
	if err := badMap.Validate(); err == nil {
		t.Error("bad map value: want error, got nil")
	}
}
