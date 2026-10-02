package crawlingest

import (
	"context"
	"log"

	"github.com/thg/scraper/internal/leadingest"
	"github.com/thg/scraper/internal/models"
	"github.com/thg/scraper/internal/services/facebook"
	"github.com/thg/scraper/internal/telegram/control"
)

// notifyCrawlLead delivers the single completed enrichment snapshot to both
// sinks. The durable CRM outbox is queued before the optional Telegram notice;
// neither sink regenerates AI from the raw lead.
func (h *Handler) notifyCrawlLead(ev leadingest.LeadEvent) {
	workspace := ""
	if org, _ := h.db.GetOrganization(ev.OrgID); org != nil {
		workspace = org.Name
	}
	deliver := func(suggestion models.LeadSuggestion) {
		if h.crmLeadSync != nil {
			if err := h.crmLeadSync.Enqueue(context.Background(), ev, suggestion); err != nil {
				log.Printf("CRM enriched lead enqueue failed: %v", err)
			}
		}
		if h.tgEvents == nil {
			return
		}
		h.tgEvents.NotifyLead(control.LeadNotice{
			OrgID: ev.OrgID, LeadID: ev.LeadID, Channel: "facebook", Workspace: workspace,
			Author: ev.AuthorName, PostURL: ev.PostURL, Excerpt: ev.Excerpt, Reason: ev.Reason, BaseURL: h.baseURL,
			SuggestedReply: suggestion.Reply, ProductName: suggestion.ProductName, ProductURL: suggestion.ProductURL,
			ProductImageURL: suggestion.ProductImageURL,
			Supplier:        suggestion.Supplier, SourcingNote: suggestion.SourcingNote,
		})
	}
	if h.leadSuggestion != nil && h.leadSuggestionAllowed != nil && h.leadSuggestionAllowed(ev.OrgID) {
		unavailable := facebook.UnavailableLeadSuggestion(ev.Excerpt, ev.AuthorName)
		if h.suggestionRunner != nil && h.suggestionRunner.TryWithFallback(
			func(ctx context.Context) models.LeadSuggestion { return h.leadSuggestion(ctx, ev) }, deliver, unavailable,
		) {
			return
		}
		deliver(unavailable)
		return
	}
	deliver(models.LeadSuggestion{})
}
