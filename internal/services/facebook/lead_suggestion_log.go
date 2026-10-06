package facebook

import (
	"log"
	"strings"

	"github.com/thg/scraper/internal/models"
)

// Suggestion gates recorded per lead, so a notice without "Gợi ý trả lời"
// can be explained from the service journal.
const (
	SuggestionGateDisabled   = "disabled"
	SuggestionGateNotAllowed = "org_not_allowed"
	SuggestionGateBusy       = "runner_busy"
	SuggestionGateAttempted  = "attempted"
)

// LogLeadSuggestionOutcome records why a lead notice has or lacks a draft.
// It logs no post text, author or reply: only the org, the gate, whether a
// reply exists and the fixed operator status note.
func LogLeadSuggestionOutcome(path string, orgID int64, gate string, suggestion models.LeadSuggestion) {
	log.Printf("[LeadSuggestion] path=%s org_id=%d gate=%s reply=%t supplier=%t product=%t note=%q",
		path, orgID, gate, strings.TrimSpace(suggestion.Reply) != "", suggestion.Supplier.HasOffer(),
		strings.TrimSpace(suggestion.ProductURL) != "", suggestion.SourcingNote)
}
