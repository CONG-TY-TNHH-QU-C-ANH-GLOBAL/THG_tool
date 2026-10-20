package config

import (
	"net/url"
	"strings"
)

// LeadModelConfig isolates operator suggestions from outbound comments. An
// explicit but incomplete override must never borrow another provider's key.
type LeadModelConfig struct {
	APIKey, BaseURL, Model string
}

func loadLeadModel() LeadModelConfig {
	return LeadModelConfig{
		APIKey:  strings.TrimSpace(getEnv("LEAD_MODEL_API_KEY", "")),
		BaseURL: strings.TrimRight(strings.TrimSpace(getEnv("LEAD_MODEL_BASE_URL", "")), "/"),
		Model:   strings.TrimSpace(getEnv("LEAD_MODEL_NAME", "")),
	}
}

func (c LeadModelConfig) Explicit() bool {
	return c.APIKey != "" || c.BaseURL != "" || c.Model != ""
}

func (c LeadModelConfig) Complete() bool {
	u, err := url.Parse(c.BaseURL)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && c.APIKey != "" && c.Model != ""
}
