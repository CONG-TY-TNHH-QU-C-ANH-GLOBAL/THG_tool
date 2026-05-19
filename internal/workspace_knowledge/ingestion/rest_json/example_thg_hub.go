package rest_json

import (
	"encoding/json"

	"github.com/thg/scraper/internal/workspace_knowledge/products"
)

// ExampleConfigTHGHub returns the connection_config blob for one
// well-known catalog backend — THG Fulfill's public hub.
//
// IMPORTANT: this is NOT a hardcoded vendor. The adapter does not
// branch on tenant or hub host anywhere. This function exists only
// as documentation + test fixture: it shows how the generic config
// maps to one real upstream so reviewers can read a concrete example
// and so the integration tests can exercise the same path operators
// will use in the UI.
//
// In production, operators construct the equivalent JSON through the
// Connect Product Catalog wizard (PR-4). The output of that wizard
// goes into knowledge_sources.connection_config as opaque JSON; the
// adapter then reads it back via ParseConfig.
//
// Preset reuse: when multiple tenants want to connect the same backend
// (e.g. several sellers using the same fulfilment hub), the wizard
// can offer "saved configs" as a dropdown. Saved configs live in a
// future adapter_presets table — they are DATA, not Go code, and the
// adapter still does not know they exist.
func ExampleConfigTHGHub() json.RawMessage {
	cfg := Config{
		BaseURL:          "https://hub.thgfulfill.com/api/public/catalog",
		ExtractorVersion: "rest_json/v1",
		Request: RequestConfig{
			TimeoutSeconds: 30,
			UserAgent:      "THGKnowledgeIngestor/1.0",
		},
		Auth: AuthConfig{Type: "none"},
		Pagination: PaginationConfig{
			Scheme:         "page",
			PageParam:      "page",
			LimitParam:     "limit",
			LimitValue:     100,
			StartPage:      1,
			TotalPagesPath: "pagination.pages",
			MaxPages:       10,
		},
		DataPath: "data",
		FieldMap: FieldMap{
			SourceID:          "id",
			DisplaySKU:        "thgSku",
			VendorSKU:         "sku",
			Name:              "name",
			Description:       "", // currently empty upstream — leave unmapped
			Category:          "category",
			Origin:            "origin",
			Sizes:             "sizes",
			Colors:            "colors",
			Tags:              "",
			PriceMin:          "priceFrom",
			PriceMax:          "priceTo",
			Currency:          "currency",
			Images:            "images",
			SourceURLTemplate: "https://www.thgfulfill.com/catalog/{id}",
			SourceUpdatedAt:   "updatedAt",
		},
		Availability: AvailabilityConfig{
			FromField: "status",
			Map: map[string]products.Availability{
				"Active":   products.AvailInStock,
				"Inactive": products.AvailOutOfStock,
			},
			Default: products.AvailUnknown,
		},
	}
	raw, _ := json.Marshal(cfg)
	return raw
}
