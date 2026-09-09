package supplier_catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/thg/scraper/internal/suppliersourcing"
)

// Config is the connection_config for a supplier_catalog source.
//
// The operator declares WHAT to index, never how many upstream calls to spend:
// MaxAPICalls is the hard ceiling for one sync run, because the upstream
// marketplace budget is small, shared, and monthly. A run stops cleanly at the
// ceiling and reports how far it got.
type Config struct {
	// BaseURL is the THG Pricing Hub origin (it owns the marketplace API key).
	BaseURL string `json:"base_url"`
	// SecretEnv or SecretFile supplies the integration key. Exactly one.
	SecretEnv  string `json:"secret_env"`
	SecretFile string `json:"secret_file"`

	// Queries are keyword searches to discover items to index.
	Queries []Query `json:"queries"`
	// Links are explicit marketplace product URLs the operator already picked.
	// These cost one upstream call each and skip the discovery step.
	Links []string `json:"links"`

	// MaxAPICalls caps upstream calls for one run. Required and positive.
	MaxAPICalls int `json:"max_api_calls"`
	// DetailPerQuery limits how many search hits per query get promoted to a
	// full detail lookup (a detail call is what yields weight and MOQ).
	DetailPerQuery int `json:"detail_per_query,omitempty"`
	// TimeoutSeconds bounds each upstream call.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// Tags are attached to every asset this source writes.
	Tags []string `json:"tags,omitempty"`
}

// Query is one keyword lookup against one marketplace.
type Query struct {
	Q        string `json:"q"`
	Platform string `json:"platform"` // 1688 | alibaba | taobao | tmall
	Size     int    `json:"size,omitempty"`
}

func parseConfig(raw json.RawMessage) (Config, error) {
	var cfg Config
	if len(raw) == 0 || json.Unmarshal(raw, &cfg) != nil {
		return cfg, errors.New("supplier_catalog: invalid connection_config")
	}
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.SecretEnv = strings.TrimSpace(cfg.SecretEnv)
	cfg.SecretFile = strings.TrimSpace(cfg.SecretFile)
	if !strings.HasPrefix(cfg.BaseURL, "https://") {
		return cfg, errors.New("supplier_catalog: base_url must be https")
	}
	if (cfg.SecretEnv == "") == (cfg.SecretFile == "") {
		return cfg, errors.New("supplier_catalog: configure secret_env or secret_file, not both")
	}
	for index := range cfg.Queries {
		platform := suppliersourcing.NormalizePlatform(cfg.Queries[index].Platform)
		if platform == "" {
			return cfg, fmt.Errorf("supplier_catalog: invalid platform %q", cfg.Queries[index].Platform)
		}
		cfg.Queries[index].Platform = platform
		cfg.Queries[index].Q = strings.TrimSpace(cfg.Queries[index].Q)
		if cfg.Queries[index].Q == "" {
			return cfg, errors.New("supplier_catalog: query text is required")
		}
		if cfg.Queries[index].Size <= 0 {
			cfg.Queries[index].Size = 20
		}
	}
	cfg.Links = trimmedNonEmpty(cfg.Links)
	if len(cfg.Queries) == 0 && len(cfg.Links) == 0 {
		return cfg, errors.New("supplier_catalog: at least one query or link is required")
	}
	if cfg.MaxAPICalls <= 0 {
		return cfg, errors.New("supplier_catalog: max_api_calls must be positive")
	}
	if cfg.DetailPerQuery <= 0 {
		cfg.DetailPerQuery = 5
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 20
	}
	return cfg, nil
}

func loadSecret(cfg Config) (string, error) {
	if cfg.SecretEnv != "" {
		if secret := strings.TrimSpace(os.Getenv(cfg.SecretEnv)); secret != "" {
			return secret, nil
		}
		return "", fmt.Errorf("supplier_catalog: %s is empty", cfg.SecretEnv)
	}
	contents, err := os.ReadFile(cfg.SecretFile)
	if err != nil {
		return "", fmt.Errorf("supplier_catalog: read secret_file: %w", err)
	}
	if secret := strings.TrimSpace(string(contents)); secret != "" {
		return secret, nil
	}
	return "", errors.New("supplier_catalog: secret_file is empty")
}

func trimmedNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
