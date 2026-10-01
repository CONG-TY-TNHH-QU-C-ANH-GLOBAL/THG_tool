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
	if !allowlist.Configured() {
		log.Println("[LeadSuggestion] enabled but LEAD_SUGGESTION_ORG_IDS is empty/invalid; no org is allowed")
	}
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
		return facebook.BuildLeadSuggestion(ctx, builder, commentGen, ai.LoadProfileForOrg(db, ev.OrgID), ev.OrgID, ev.Excerpt, ev.AuthorName, shippingQuote, supplierLookup)
	}
	runner := notifications.NewSuggestionRunner(cfg.LeadSuggestionMaxConcurrency, time.Duration(cfg.LeadSuggestionTimeoutMS)*time.Millisecond)
	return build, allowlist.Allows, runner
}
