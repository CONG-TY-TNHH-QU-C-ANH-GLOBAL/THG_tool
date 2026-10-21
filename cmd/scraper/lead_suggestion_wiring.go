package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/crmleadsync"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/notifications"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
	"github.com/thg/scraper/internal/suppliersourcing"
	knowledgeRuntime "github.com/thg/scraper/internal/workspace_knowledge/runtime"
)

// setupLeadSuggestionRuntime is composition-root wiring: the Facebook service
// owns grounding/generation policy, while neutral server packages receive only
// notification contracts and callbacks.
func setupLeadSuggestionRuntime(cfg *config.Config, db *store.Store, commentGen *ai.MessageGenerator) (leadingest.SuggestionBuild, func(int64) bool, *notifications.SuggestionRunner) {
	if cfg == nil || db == nil || !cfg.LeadSuggestionEnabled {
		return nil, nil, nil
	}
	allowlist := facebook.ParseOrgAllowlist(cfg.LeadSuggestionOrgIDs)
	contextAllowlist := facebook.ParseOrgAllowlist(cfg.LeadContextOrgIDs)
	if !allowlist.Configured() {
		log.Println("[LeadSuggestion] enabled but LEAD_SUGGESTION_ORG_IDS is empty/invalid; no org is allowed")
	}
	commentGen = leadSuggestionGenerator(cfg, commentGen)
	if commentGen == nil || !commentGen.Available() {
		log.Println("[LeadSuggestion] comment provider unavailable; grounded product suggestions use concise fallback copy")
	}
	builder := knowledgeRuntime.NewBuilder(db.Knowledge())
	shippingURL := strings.TrimSpace(os.Getenv("CRM_SHIPPING_QUOTE_URL"))
	if shippingURL == "" {
		shippingURL = crmleadsync.DefaultShippingQuoteURL
	}
	shippingKey := crmLeadSyncSecret()
	shippingQuote := func(ctx context.Context, shipment models.ShippingRequest) (*models.ShippingReference, error) {
		return crmleadsync.QuoteSupplierShipment(ctx, shippingURL, shippingKey, shipment)
	}
	var supplierLookup facebook.SupplierLookupFunc
	if pricingClient := suppliersourcing.RuntimeClient(); pricingClient != nil {
		supplierLookup = facebook.NewSupplierLookup(pricingClient)
	}
	build := func(ctx context.Context, ev leadingest.LeadEvent) models.LeadSuggestion {
		if !allowlist.Allows(ev.OrgID) {
			return models.LeadSuggestion{}
		}
		images := ev.ImageURLs
		if !cfg.LeadImageContextEnabled {
			images = nil
		}
		switch {
		case !contextAllowlist.Allows(ev.OrgID):
			// Keep the existing text suggestion path outside the analysis scope.
		case cfg.LeadContextAnalysisEnabled:
			return facebook.BuildLeadSuggestionWithContext(ctx, builder, commentGen, ai.LoadProfileForOrg(db, ev.OrgID), ev.OrgID, ev.Excerpt, ev.AuthorName, images, shippingQuote, supplierLookup)
		case cfg.LeadImageContextEnabled:
			return facebook.BuildLeadSuggestionWithImages(ctx, builder, commentGen, ai.LoadProfileForOrg(db, ev.OrgID), ev.OrgID, ev.Excerpt, ev.AuthorName, ev.ImageURLs, shippingQuote, supplierLookup)
		}
		return facebook.BuildLeadSuggestion(ctx, builder, commentGen, ai.LoadProfileForOrg(db, ev.OrgID), ev.OrgID, ev.Excerpt, ev.AuthorName, shippingQuote, supplierLookup)
	}
	baseTimeout := time.Duration(cfg.LeadSuggestionTimeoutMS) * time.Millisecond
	analysisEnabled := cfg.LeadContextAnalysisEnabled || cfg.LeadImageContextEnabled
	queueScope := contextAllowlist.Allows
	if !analysisEnabled {
		queueScope = nil
	}
	runner := notifications.NewQueuedSuggestionRunner(cfg.LeadSuggestionMaxConcurrency, baseTimeout, queueScope, func(orgID int64) time.Duration {
		return facebook.LeadSuggestionTimeout(baseTimeout, analysisEnabled, contextAllowlist, orgID)
	})
	return build, allowlist.Allows, runner
}

func leadSuggestionGenerator(cfg *config.Config, fallback *ai.MessageGenerator) *ai.MessageGenerator {
	if !cfg.LeadModel.Explicit() {
		return fallback
	}
	if !cfg.LeadModel.Complete() {
		return nil
	}
	return ai.NewMessageGeneratorWithEndpoint(cfg.LeadModel.APIKey, cfg.LeadModel.Model, cfg.LeadModel.BaseURL)
}
