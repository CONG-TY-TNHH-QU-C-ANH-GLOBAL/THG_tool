package main

import (
	"context"
	"log"
	"time"

	"github.com/thg/scraper/internal/ai"
	"github.com/thg/scraper/internal/config"
	"github.com/thg/scraper/internal/crmenrich"
	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/notifications"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/store"
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
		log.Println("[LeadSuggestion] enabled but comment provider is unavailable; suggestions remain inactive")
		return nil, nil, nil
	}
	builder := knowledgeRuntime.NewBuilder(db.Knowledge())
	// Same wiring as the worker: the CRM holds the published rate cards and the
	// marketplace product reader, so the figures come from there. nil when the
	// key is absent, and the suggestion is then exactly what it was before.
	crm := crmenrich.New(crmEnrichURL(), crmLeadSyncSecret())
	if crm.Available() {
		log.Println("[LeadSuggestion] CRM enrich wired — replies may carry product price and shipping cost")
	}
	build := func(ctx context.Context, ev leadingest.LeadEvent) models.LeadSuggestion {
		if !allowlist.Allows(ev.OrgID) {
			return models.LeadSuggestion{}
		}
		return facebook.BuildLeadSuggestionWithCRM(
			ctx, builder, commentGen, ai.LoadProfileForOrg(db, ev.OrgID), crm,
			ev.OrgID, ev.Excerpt, ev.AuthorName, ev.PostURL, ev.Category,
		)
	}
	runner := notifications.NewSuggestionRunner(cfg.LeadSuggestionMaxConcurrency, time.Duration(cfg.LeadSuggestionTimeoutMS)*time.Millisecond)
	return build, allowlist.Allows, runner
}
