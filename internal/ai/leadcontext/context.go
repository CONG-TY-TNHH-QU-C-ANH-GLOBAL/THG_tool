// Package leadcontext owns tentative need analysis over a post and its images.
// It selects no supplier and cannot authorize business or shipping claims.
package leadcontext

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/thg/scraper/internal/ai/visualcontext"
)

const (
	Sourcing      = "product_sourcing"
	POD           = "pod"
	Shipping      = "shipping"
	Warehouse     = "warehousing"
	NonCommercial = "no_commercial_need"
	Unclear       = "unclear"
)

type Context struct {
	Intent        string                `json:"intent"`
	Product       string                `json:"product"`
	Evidence      []string              `json:"text_evidence"`
	Missing       []string              `json:"missing_fields"`
	Visual        visualcontext.Context `json:"visual"`
	ImageEvidence []ImageEvidence       `json:"image_evidence,omitempty"`
	Scope         *ScopeClarification   `json:"scope_clarification,omitempty"`
}

// Parse requires literal evidence from the supplied post. Model interpretation
// remains tentative even when the response is structurally valid.
func Parse(data []byte, post string, hasImages bool) (Context, error) {
	imageCount := 0
	if hasImages {
		imageCount = 3 // Legacy boolean API only knows that up to three images exist.
	}
	return ParseWithImages(data, post, imageCount)
}

// ParseWithImages binds every image quotation to the actual request's photo
// count. Quotations remain model observations, not verified numeric facts.
func ParseWithImages(data []byte, post string, imageCount int) (Context, error) {
	var fields map[string]json.RawMessage
	if imageCount < 0 || imageCount > 3 || json.Unmarshal(data, &fields) != nil || !validContextKeys(fields) {
		return Context{}, errors.New("invalid lead context")
	}
	for _, key := range []string{"intent", "product", "text_evidence", "missing_fields", "visual"} {
		if len(fields[key]) == 0 || string(fields[key]) == "null" {
			return Context{}, errors.New("incomplete lead context")
		}
	}
	var out Context
	if json.Unmarshal(data, &out) != nil || !validIntent(out.Intent) || !validProduct(out.Product) || !validMissing(out.Missing) {
		return Context{}, errors.New("invalid lead interpretation")
	}
	visual, err := visualcontext.Parse(fields["visual"])
	if err != nil || (imageCount == 0 && (visual.Product != "" || len(visual.Observations) > 0 || visual.Conflict)) {
		return Context{}, errors.New("unsupported image observation")
	}
	if !validImageEvidence(fields["image_evidence"], imageCount, visual.Observations) {
		return Context{}, errors.New("invalid image request evidence")
	}
	if !validEvidence(out.Evidence, post) || lacksNeedEvidence(out) || !validScopeClarification(fields["scope_clarification"], out, post, imageCount) {
		return Context{}, errors.New("lead interpretation lacks grounded request or clarification evidence")
	}
	out.Product, out.Visual = strings.TrimSpace(out.Product), visual
	return out, nil
}

func validContextKeys(fields map[string]json.RawMessage) bool {
	if len(fields) < 5 || len(fields) > 7 {
		return false
	}
	for key := range fields {
		switch key {
		case "intent", "product", "text_evidence", "missing_fields", "visual", "image_evidence", "scope_clarification":
		default:
			return false
		}
	}
	return true
}

func lacksNeedEvidence(c Context) bool {
	return c.Intent != Unclear && len(c.Evidence) == 0 && len(c.ImageEvidence) == 0
}

func validIntent(value string) bool {
	switch value {
	case Sourcing, POD, Shipping, Warehouse, NonCommercial, Unclear:
		return true
	}
	return false
}

func validProduct(value string) bool {
	return visualcontext.ValidProductHint(value)
}

func validEvidence(values []string, post string) bool {
	if len(values) > 4 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len([]rune(value)) > 240 || !strings.Contains(post, value) {
			return false
		}
	}
	return true
}

func validMissing(values []string) bool {
	if len(values) > 6 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		switch value {
		case "product", "quantity", "origin", "destination", "parcel_dimensions_weight", "customization_artwork":
		default:
			return false
		}
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func (c Context) PromptBlock() string {
	data, _ := json.Marshal(c)
	return string(data)
}
